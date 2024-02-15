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

package rclone

import (
	"context"
	"errors"
	"path"

	"github.com/go-logr/logr"
	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	vserrors "github.com/backube/volsync/controllers/errors"
	"github.com/backube/volsync/controllers/mover"
	"github.com/backube/volsync/controllers/utils"
	"github.com/backube/volsync/controllers/volumehandler"
)

const (
	mountPath         = "/data"
	dataVolumeName    = "data"
	rcloneSecret      = "rclone-secret"
	rcloneCAMountPath = "/customCA"
	rcloneCAFilename  = "ca.crt"
)

// Mover is the reconciliation logic for the Rclone-based data mover.
type Mover struct {
	client              client.Client
	logger              logr.Logger
	eventRecorder       events.EventRecorder
	owner               client.Object
	vh                  *volumehandler.VolumeHandler
	saHandler           utils.SAHandler
	containerImage      string
	rcloneConfigSection *string
	rcloneDestPath      *string
	rcloneConfig        *string
	isSource            bool
	paused              bool
	mainPVCName         *string
	customCASpec        volsyncv1alpha1.CustomCASpec
	privileged          bool // true if the mover should have elevated privileges
	latestMoverStatus   *volsyncv1alpha1.MoverStatus
	moverConfig         volsyncv1alpha1.MoverConfig
}

var _ mover.Mover = &Mover{}

// All object types that are temporary/per-iteration should be listed here. The
// individual objects to be cleaned up must also be marked.
var cleanupTypes = []client.Object{
	&corev1.PersistentVolumeClaim{},
	&snapv1.VolumeSnapshot{},
	&batchv1.Job{},
}

func (m *Mover) Name() string { return "rclone" }

func (m *Mover) Synchronize(ctx context.Context) (mover.Result, error) {
	var err error

	err = m.validateSpec()
	if err != nil {
		return mover.InProgress(), err
	}

	// Validate rCloneConfig Secret
	rcloneConfigSecret, err := m.validateRcloneConfig(ctx)
	if rcloneConfigSecret == nil || err != nil {
		return mover.InProgress(), err
	}

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

	// Prepare ServiceAccount, role, rolebinding
	sa, err := m.saHandler.Reconcile(ctx, m.logger)
	if sa == nil || err != nil {
		return mover.InProgress(), err
	}

	// Validate custom CA if in spec
	customCAObj, err := utils.ValidateCustomCA(ctx, m.client, m.logger,
		m.owner.GetNamespace(), m.customCASpec)
	// nil customCAObj is ok (indicates we're not using a custom CA)
	if err != nil {
		return mover.InProgress(), err
	}

	// Start mover Job
	job, err := m.ensureJob(ctx, dataPVC, sa, rcloneConfigSecret, customCAObj)
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
	dataName := mover.VolSyncPrefix + m.owner.GetName() + "-src"
	pvc, err := m.vh.EnsurePVCFromSrc(ctx, m.logger, srcPVC, dataName, true)
	if err != nil {
		// If the error was a copy TriggerTimeoutError, update the latestMoverStatus to indicate error
		var copyTriggerTimeoutError *vserrors.CopyTriggerTimeoutError
		if errors.As(err, &copyTriggerTimeoutError) {
			utils.UpdateMoverStatusFailed(m.latestMoverStatus, copyTriggerTimeoutError.Error())
			// Don't return error - we want to keep reconciling at the normal in-progress rate
			// but just indicate in the latestMoverStatus that there is an error (we've been waiting
			// for the user to update the copy Trigger for too long)
			return pvc, nil
		}
	}
	return pvc, nil
}

// this is so far is common to rclone & restic
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
		newPvcName := mover.VolSyncPrefix + m.owner.GetName() + "-dest"
		return false, newPvcName
	}
	return true, *m.mainPVCName
}

//nolint:funlen
func (m *Mover) ensureJob(ctx context.Context, dataPVC *corev1.PersistentVolumeClaim,
	sa *corev1.ServiceAccount, rcloneConfigSecret *corev1.Secret,
	customCAObj utils.CustomCAObject) (*batchv1.Job, error) {
	dir := "dst"
	direction := "destination"

	readOnlyVolume := false
	if m.isSource {
		dir = "src"
		direction = "source"

		// Set read-only for volume in source mover job spec if the PVC only supports read-only
		readOnlyVolume = utils.PvcIsReadOnly(dataPVC)
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mover.VolSyncPrefix + "rclone-" + dir + "-" + m.owner.GetName(),
			Namespace: m.owner.GetNamespace(),
		},
	}
	logger := m.logger.WithValues("job", client.ObjectKeyFromObject(job))

	_, err := utils.CreateOrUpdateDeleteOnImmutableErr(ctx, m.client, job, logger, func() error {
		if err := ctrl.SetControllerReference(m.owner, job, m.client.Scheme()); err != nil {
			logger.Error(err, utils.ErrUnableToSetControllerRef)
			return err
		}
		utils.SetOwnedByVolSync(job)
		utils.MarkForCleanup(m.owner, job)
		job.Spec.Template.ObjectMeta.Name = job.Name
		utils.SetOwnedByVolSync(&job.Spec.Template)
		backoffLimit := int32(2) //TODO: backofflimit was 8 for restic
		job.Spec.BackoffLimit = &backoffLimit

		parallelism := int32(1)
		if m.paused {
			parallelism = int32(0)
		}
		job.Spec.Parallelism = &parallelism

		envVars := []corev1.EnvVar{}
		// Rclone env vars if they are in the secret
		envVars = utils.AppendRCloneEnvVars(rcloneConfigSecret, envVars)

		defaultEnvVars := []corev1.EnvVar{
			{Name: "RCLONE_CONFIG", Value: "/rclone-config/rclone.conf"},
			{Name: "RCLONE_DEST_PATH", Value: *m.rcloneDestPath},
			{Name: "DIRECTION", Value: direction},
			{Name: "MOUNT_PATH", Value: mountPath},
			{Name: "RCLONE_CONFIG_SECTION", Value: *m.rcloneConfigSection},
		}

		// Add our defaults after RCLONE_ env vars so any duplicates will be
		// overridden by the defaults
		envVars = append(envVars, defaultEnvVars...)

		// Cluster-wide proxy settings
		envVars = utils.AppendEnvVarsForClusterWideProxy(envVars)

		job.Spec.Template.Spec.Containers = []corev1.Container{{
			Name:    "rclone",
			Env:     envVars,
			Command: []string{"/bin/bash", "-c", "/mover-rclone/active.sh"},
			Image:   m.containerImage,
			SecurityContext: &corev1.SecurityContext{
				AllowPrivilegeEscalation: ptr.To(false),
				Capabilities: &corev1.Capabilities{
					Drop: []corev1.Capability{"ALL"},
				},
				Privileged:             ptr.To(false),
				ReadOnlyRootFilesystem: ptr.To(true),
			},
			VolumeMounts: []corev1.VolumeMount{
				{Name: dataVolumeName, MountPath: mountPath},
				{Name: rcloneSecret, MountPath: "/rclone-config/"},
				{Name: "tempdir", MountPath: "/tmp"},
			},
		}}
		job.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyNever
		job.Spec.Template.Spec.ServiceAccountName = sa.Name
		job.Spec.Template.Spec.Volumes = []corev1.Volume{
			{Name: dataVolumeName, VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: dataPVC.Name,
					ReadOnly:  readOnlyVolume,
				}},
			},
			{Name: rcloneSecret, VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  rcloneConfigSecret.Name,
					DefaultMode: ptr.To[int32](0600),
				}},
			},
			{Name: "tempdir", VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{
					Medium: corev1.StorageMediumMemory,
				}},
			},
		}
		if m.vh.IsCopyMethodDirect() {
			affinity, err := utils.AffinityFromVolume(ctx, m.client, logger, dataPVC)
			if err != nil {
				logger.Error(err, "unable to determine proper affinity", "PVC", client.ObjectKeyFromObject(dataPVC))
				return err
			}
			job.Spec.Template.Spec.NodeSelector = affinity.NodeSelector
			job.Spec.Template.Spec.Tolerations = affinity.Tolerations
		}
		logger.V(1).Info("Job has PVC", "PVC", dataPVC, "DS", dataPVC.Spec.DataSource)

		podSpec := &job.Spec.Template.Spec

		if customCAObj != nil {
			// Tell mover where to find the cert
			podSpec.Containers[0].Env = append(podSpec.Containers[0].Env, corev1.EnvVar{
				Name:  "CUSTOM_CA",
				Value: path.Join(rcloneCAMountPath, rcloneCAFilename),
			})
			// Mount the custom CA certificate
			podSpec.Containers[0].VolumeMounts =
				append(podSpec.Containers[0].VolumeMounts, corev1.VolumeMount{
					Name:      "custom-ca",
					MountPath: rcloneCAMountPath,
				})
			podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
				Name:         "custom-ca",
				VolumeSource: customCAObj.GetVolumeSource(rcloneCAFilename),
			})
		}

		// Update the job securityContext, podLabels and resourceRequirements from moverConfig (if specified)
		utils.UpdatePodTemplateSpecFromMoverConfig(&job.Spec.Template, m.moverConfig, corev1.ResourceRequirements{})

		// Adjust the Job based on whether the mover should be running as privileged
		logger.Info("mover permissions", "privileged-mover", m.privileged)
		if m.privileged {
			podSpec.Containers[0].Env = append(podSpec.Containers[0].Env, corev1.EnvVar{
				Name:  "PRIVILEGED_MOVER",
				Value: "1",
			})
			podSpec.Containers[0].SecurityContext.Capabilities.Add = []corev1.Capability{
				"DAC_OVERRIDE", // Read/write all files
				"CHOWN",        // chown files
				"FOWNER",       // Set permission bits & times
			}
			podSpec.Containers[0].SecurityContext.RunAsUser = ptr.To[int64](0)
		} else {
			podSpec.Containers[0].Env = append(podSpec.Containers[0].Env, corev1.EnvVar{
				Name:  "PRIVILEGED_MOVER",
				Value: "0",
			})
		}

		return nil
	})
	// If Job had failed, delete it so it can be recreated
	if job.Status.Failed >= *job.Spec.BackoffLimit {
		// Update status with mover logs from failed job
		utils.UpdateMoverStatusForFailedJob(ctx, m.logger, m.latestMoverStatus, job.GetName(), job.GetNamespace(),
			utils.AllLines)

		logger.Info("deleting job -- backoff limit reached")
		err = m.client.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground))
		return nil, err
	}
	if err != nil {
		logger.Error(err, "reconcile failed")
		return nil, err
	}
	// Stop here if the job hasn't completed yet
	if job.Status.Succeeded == 0 {
		return nil, nil
	}

	logger.Info("job completed")

	// update status with mover logs from successful job
	utils.UpdateMoverStatusForSuccessfulJob(ctx, m.logger, m.latestMoverStatus, job.GetName(), job.GetNamespace(),
		LogLineFilterSuccess)

	// We only continue reconciling if the rclone job has completed
	return job, nil
}

func (m *Mover) validateSpec() error {
	m.logger.V(1).Info("Initiate Rclone Spec validation")
	if m.rcloneConfig == nil || len(*m.rcloneConfig) == 0 {
		err := errors.New("unable to get Rclone config secret name")
		m.logger.Error(err, "Rclone Spec validation error")
		return err
	}
	if m.rcloneConfigSection == nil || len(*m.rcloneConfigSection) == 0 {
		err := errors.New("unable to get Rclone config section name")
		m.logger.Error(err, "Rclone Spec validation error")
		return err
	}
	if m.rcloneDestPath == nil || len(*m.rcloneDestPath) == 0 {
		err := errors.New("unable to get Rclone destination name")
		m.logger.Error(err, "Rclone Spec validation error")
		return err
	}
	m.logger.V(1).Info("Rclone Spec validation complete.")
	return nil
}

func (m *Mover) validateRcloneConfig(ctx context.Context) (*corev1.Secret, error) {
	// Validate user provided rcloneConfig Secret exists and has the proper field
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      *m.rcloneConfig,
			Namespace: m.owner.GetNamespace(),
		},
	}
	logger := m.logger.WithValues("rcloneConfig Secret", client.ObjectKeyFromObject(secret))

	if err := utils.GetAndValidateSecret(ctx, m.client, logger, secret, "rclone.conf"); err != nil {
		logger.Error(err, "Rclone config secret does not contain the proper fields")
		return nil, err
	}
	m.logger.Info("RcloneConfig reconciled")
	return secret, nil
}
