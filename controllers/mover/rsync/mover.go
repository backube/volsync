/*
Copyright 2021 The VolSync authors.

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package rsync

import (
	"context"
	"strconv"

	"github.com/go-logr/logr"
	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/backube/volsync/controllers/mover"
	"github.com/backube/volsync/controllers/utils"
	"github.com/backube/volsync/controllers/volumehandler"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

const (
	mountPath      = "/data"
	dataVolumeName = "data"
)

// Mover is the reconciliation logic for the Restic-based data mover.
type Mover struct {
	client         client.Client
	logger         logr.Logger
	eventRecorder  events.EventRecorder
	owner          metav1.Object
	vh             *volumehandler.VolumeHandler
	containerImage string
	sshKeys        *string
	serviceType    *corev1.ServiceType
	address        *string
	port           *int32
	isSource       bool
	paused         bool
	mainPVCName    *string
	sourceStatus   *volsyncv1alpha1.ReplicationSourceRsyncStatus
	destStatus     *volsyncv1alpha1.ReplicationDestinationRsyncStatus
}

var _ mover.Mover = &Mover{}

// All object types that are temporary/per-iteration should be listed here. The
// individual objects to be cleaned up must also be marked.
var cleanupTypes = []client.Object{
	&corev1.PersistentVolumeClaim{},
	&snapv1.VolumeSnapshot{},
	&batchv1.Job{},
}

func (m *Mover) Name() string { return "rsync" }

func (m *Mover) Synchronize(ctx context.Context) (mover.Result, error) {
	var err error

	// Allocate temporary data PVC
	var dataPVC *corev1.PersistentVolumeClaim
	if m.isSource {
		dataPVC, err = m.ensureSourcePVC(ctx)
	} else {
		dataPVC, err = m.ensureDestinationPVC(ctx)
	}
	if dataPVC == nil || err != nil {
		return mover.InProgress(), err
	}

	// Ensure service (if required) and publish the address in the status
	cont, err := m.ensureServiceAndPublishAddress(ctx)
	if !cont || err != nil {
		return mover.InProgress(), err
	}

	// Ensure Secrets/keys
	rsyncSecretName, err := m.ensureSecrets(ctx)
	if rsyncSecretName == nil || err != nil {
		return mover.InProgress(), err
	}

	// Prepare ServiceAccount, role, rolebinding
	sa, err := m.ensureSA(ctx)
	if sa == nil || err != nil {
		return mover.InProgress(), err
	}

	// Ensure mover Job
	job, err := m.ensureJob(ctx, dataPVC, sa, *rsyncSecretName)
	if job == nil || err != nil {
		return mover.InProgress(), err
	}

	// On the destination, preserve the image and return it
	if !m.isSource {
		image, err := m.vh.EnsureImage(ctx, m.logger, dataPVC)
		if image == nil || err != nil {
			return mover.InProgress(), err
		}
		return mover.CompleteWithImage(image), nil
	}

	// On the source, just signal completion
	return mover.Complete(), nil
}

func (m *Mover) ensureServiceAndPublishAddress(ctx context.Context) (bool, error) {
	if m.address != nil {
		// Connection will be outbound. Don't need a Service
		return true, nil
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "volsync-rsync-" + m.direction() + "-" + m.owner.GetName(),
			Namespace: m.owner.GetNamespace(),
		},
	}
	svcDesc := rsyncSvcDescription{
		Context:  ctx,
		Client:   m.client,
		Service:  service,
		Owner:    m.owner,
		Type:     m.serviceType,
		Selector: m.serviceSelector(),
		Port:     m.port,
	}
	err := svcDesc.Reconcile(m.logger)
	if err != nil {
		return false, err
	}

	return m.publishSvcAddress(service)
}

func (m *Mover) publishSvcAddress(service *corev1.Service) (bool, error) {
	address := getServiceAddress(service)
	if address == "" {
		// We don't have an address yet, try again later
		m.updateStatusAddress(nil)
		return false, nil
	}
	m.updateStatusAddress(&address)

	m.logger.V(1).Info("Service addr published", "address", address)
	return true, nil
}

func (m *Mover) updateStatusAddress(address *string) {
	if m.isSource {
		m.sourceStatus.Address = address
	} else {
		m.destStatus.Address = address
	}
}

func (m *Mover) updateStatusSSHKeys(sshKeys *string) {
	if m.isSource {
		m.sourceStatus.SSHKeys = sshKeys
	} else {
		m.destStatus.SSHKeys = sshKeys
	}
}

// Will ensure the secret exists or create secrets if necessary
// - If secrets are created, will expose the appropriate secret in the status (src secret if ReplicationDestination,
//   dest secret if ReplicationSource)
// - Returns the name of the secret that should be used in the replication job
func (m *Mover) ensureSecrets(ctx context.Context) (*string, error) {
	// If user provided keys, use those
	if m.sshKeys != nil {
		rsyncSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      *m.sshKeys,
				Namespace: m.owner.GetNamespace(),
			},
		}
		fields := []string{"destination", "destination.pub", "source.pub"}
		if m.isSource {
			fields = []string{"source", "source.pub", "destination.pub"}
		}
		if err := utils.GetAndValidateSecret(ctx, m.client, m.logger, rsyncSecret, fields...); err != nil {
			m.logger.Error(err, "SSH keys secret does not contain the proper fields")
			return nil, err
		}
		return m.sshKeys, nil
	}

	// otherwise, we need to create our own
	keyInfo := rsyncSSHKeys{
		Context:      ctx,
		Client:       m.client,
		Owner:        m.owner,
		NameTemplate: "volsync-rsync-" + m.direction(),
	}
	cont, err := keyInfo.Reconcile(m.logger)
	if !cont || err != nil {
		m.updateStatusSSHKeys(nil)
		return nil, err
	}

	if m.isSource {
		// For ReplicationSource, expose dest secret in status but return src secret name (to be later used
		// in the replication source job)
		m.updateStatusSSHKeys(&keyInfo.DestSecret.Name)
		return &keyInfo.SrcSecret.Name, nil
	}
	// For ReplicationDestination, expose source secret in status but return dest secret name (to be later used
	// in the replication destination job)
	m.updateStatusSSHKeys(&keyInfo.SrcSecret.Name)
	return &keyInfo.DestSecret.Name, nil
}

func (m *Mover) direction() string {
	dir := "src"
	if !m.isSource {
		dir = "dst"
	}
	return dir
}

func (m *Mover) serviceSelector() map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":      m.direction() + "-" + m.owner.GetName(),
		"app.kubernetes.io/component": "rsync-mover",
		"app.kubernetes.io/part-of":   "volsync",
	}
}

func (m *Mover) Cleanup(ctx context.Context) (mover.Result, error) {
	m.logger.V(1).Info("Starting cleanup", "m.mainPVCName", m.mainPVCName, "m.isSource", m.isSource)
	if !m.isSource {
		m.logger.V(1).Info("removing snapshot annotations from pvc")
		// Cleanup the snapshot annotation on pvc for replicationDestination scenario so that
		// on the next sync (if snapshot CopyMethod is being used) a new snapshot will be created rather than re-using
		_, destPVCName := m.getDestinationPVCName()
		err := m.vh.RemoveSnapshotAnnotationFromPVC(ctx, m.logger, destPVCName)
		if err != nil {
			return mover.InProgress(), err
		}
	}

	err := utils.CleanupObjects(ctx, m.client, m.logger, m.owner, cleanupTypes)
	if err != nil {
		return mover.InProgress(), err
	}
	m.logger.V(1).Info("Cleanup complete")
	return mover.Complete(), nil
}

func (m *Mover) ensureSourcePVC(ctx context.Context) (*corev1.PersistentVolumeClaim, error) {
	srcPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      *m.mainPVCName,
			Namespace: m.owner.GetNamespace(),
		},
	}
	if err := m.client.Get(ctx, client.ObjectKeyFromObject(srcPVC), srcPVC); err != nil {
		m.logger.Error(err, "unable to get source PVC", "PVC", client.ObjectKeyFromObject(srcPVC))
		return nil, err
	}
	dataName := "volsync-" + m.owner.GetName() + "-" + m.direction()
	return m.vh.EnsurePVCFromSrc(ctx, m.logger, srcPVC, dataName, true)
}

func (m *Mover) ensureDestinationPVC(ctx context.Context) (*corev1.PersistentVolumeClaim, error) {
	isProvidedPVC, dataPVCName := m.getDestinationPVCName()
	if isProvidedPVC {
		return m.vh.UseProvidedPVC(ctx, dataPVCName)
	}
	// Need to allocate the incoming data volume
	return m.vh.EnsureNewPVC(ctx, m.logger, dataPVCName)
}

func (m *Mover) getDestinationPVCName() (bool, string) {
	if m.mainPVCName == nil {
		newPvcName := "volsync-" + m.owner.GetName() + "-" + m.direction()
		return false, newPvcName
	}
	return true, *m.mainPVCName
}

// this is so far is common to rclone & restic
func (m *Mover) ensureSA(ctx context.Context) (*corev1.ServiceAccount, error) {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "volsync-" + m.direction() + "-" + m.owner.GetName(),
			Namespace: m.owner.GetNamespace(),
		},
	}
	saDesc := utils.NewSAHandler(ctx, m.client, m.owner, sa)
	cont, err := saDesc.Reconcile(m.logger)
	if cont {
		return sa, err
	}
	return nil, err
}

//nolint:funlen
func (m *Mover) ensureJob(ctx context.Context, dataPVC *corev1.PersistentVolumeClaim,
	sa *corev1.ServiceAccount, rsyncSecretName string) (*batchv1.Job, error) {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "volsync-rsync-" + m.direction() + "-" + m.owner.GetName(),
			Namespace: m.owner.GetNamespace(),
		},
	}
	logger := m.logger.WithValues("job", client.ObjectKeyFromObject(job))

	op, err := ctrlutil.CreateOrUpdate(ctx, m.client, job, func() error {
		if err := ctrl.SetControllerReference(m.owner, job, m.client.Scheme()); err != nil {
			logger.Error(err, "unable to set controller reference")
			return err
		}
		utils.MarkForCleanup(m.owner, job)

		job.Spec.Template.ObjectMeta.Name = job.Name
		if job.Spec.Template.ObjectMeta.Labels == nil {
			job.Spec.Template.ObjectMeta.Labels = map[string]string{}
		}
		for k, v := range m.serviceSelector() {
			job.Spec.Template.ObjectMeta.Labels[k] = v
		}
		backoffLimit := int32(2)
		job.Spec.BackoffLimit = &backoffLimit

		parallelism := int32(1)
		if m.paused {
			parallelism = int32(0)
		}
		job.Spec.Parallelism = &parallelism

		runAsUser := int64(0)

		containerEnv := []corev1.EnvVar{}
		containerCmd := []string{"/bin/bash", "-c", "/destination.sh"} // cmd for replicationDestination job
		if m.isSource {
			// Set dest address/port if necessary
			if m.address != nil {
				containerEnv = append(containerEnv, corev1.EnvVar{Name: "DESTINATION_ADDRESS", Value: *m.address})
				if m.port != nil {
					connectPort := strconv.Itoa(int(*m.port))
					containerEnv = append(containerEnv, corev1.EnvVar{Name: "DESTINATION_PORT", Value: connectPort})
				}
			}
			// Set container cmd for the replicationSource job
			containerCmd = []string{"/bin/bash", "-c", "/source.sh"}
		}
		job.Spec.Template.Spec.Containers = []corev1.Container{{
			Name:    "rsync",
			Env:     containerEnv,
			Command: containerCmd,
			Image:   m.containerImage,
			SecurityContext: &corev1.SecurityContext{
				Capabilities: &corev1.Capabilities{
					Add: []corev1.Capability{
						"AUDIT_WRITE",
						"SYS_CHROOT",
					},
				},
				RunAsUser: &runAsUser,
			},
			VolumeMounts: []corev1.VolumeMount{
				{Name: dataVolumeName, MountPath: mountPath},
				{Name: "keys", MountPath: "/keys"},
			},
		}}
		job.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyNever
		job.Spec.Template.Spec.ServiceAccountName = sa.Name
		secretMode := int32(0600)
		job.Spec.Template.Spec.Volumes = []corev1.Volume{
			{Name: dataVolumeName, VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: dataPVC.Name,
					ReadOnly:  false,
				}},
			},
			{Name: "keys", VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  rsyncSecretName,
					DefaultMode: &secretMode,
				}},
			},
		}
		logger.V(1).Info("Job has PVC", "PVC", dataPVC, "DS", dataPVC.Spec.DataSource)
		return nil
	})
	// If Job had failed, delete it so it can be recreated
	if job.Status.Failed >= *job.Spec.BackoffLimit {
		logger.Info("deleting job -- backoff limit reached")
		err = m.client.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground))
		return nil, err
	}
	if err != nil {
		logger.Error(err, "reconcile failed")
	} else {
		logger.V(1).Info("Job reconciled", "operation", op)
	}
	// Stop here if the job hasn't completed yet
	if job.Status.Succeeded == 0 {
		return nil, nil
	}

	logger.Info("job completed")
	// We only continue reconciling if the rclone job has completed
	return job, nil
}
