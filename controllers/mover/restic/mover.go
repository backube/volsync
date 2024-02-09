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

package restic

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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
	resticCacheMountPath = "/cache"
	mountPath            = "/data"
	dataVolumeName       = "data"
	resticCache          = "cache"
	resticCAMountPath    = "/customCA"
	resticCAFilename     = "ca.crt"
	credentialDir        = "/credentials"
	gcsCredentialFile    = "gcs.json"
)

// Mover is the reconciliation logic for the Restic-based data mover.
type Mover struct {
	client                client.Client
	logger                logr.Logger
	eventRecorder         events.EventRecorder
	owner                 client.Object
	vh                    *volumehandler.VolumeHandler
	saHandler             utils.SAHandler
	containerImage        string
	cacheAccessModes      []corev1.PersistentVolumeAccessMode
	cacheCapacity         *resource.Quantity
	cacheStorageClassName *string
	repositoryName        string
	isSource              bool
	paused                bool
	mainPVCName           *string
	customCASpec          volsyncv1alpha1.CustomCASpec
	privileged            bool
	latestMoverStatus     *volsyncv1alpha1.MoverStatus
	moverConfig           volsyncv1alpha1.MoverConfig
	// Source-only fields
	pruneInterval *int32
	unlock        string
	retainPolicy  *volsyncv1alpha1.ResticRetainPolicy
	sourceStatus  *volsyncv1alpha1.ReplicationSourceResticStatus
	// Destination-only fields
	previous    *int32
	restoreAsOf *string
}

var _ mover.Mover = &Mover{}

// All object types that are temporary/per-iteration should be listed here. The
// individual objects to be cleaned up must also be marked.
var cleanupTypes = []client.Object{
	&corev1.PersistentVolumeClaim{},
	&snapv1.VolumeSnapshot{},
	&batchv1.Job{},
}

func (m *Mover) Name() string { return "restic" }

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

	// Allocate cache volume
	cachePVC, err := m.ensureCache(ctx, dataPVC)
	if cachePVC == nil || err != nil {
		return mover.InProgress(), err
	}

	// Prepare ServiceAccount
	sa, err := m.saHandler.Reconcile(ctx, m.logger)
	if sa == nil || err != nil {
		return mover.InProgress(), err
	}

	// Validate Repository Secret
	repo, err := m.validateRepository(ctx)
	if repo == nil || err != nil {
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
	job, err := m.ensureJob(ctx, cachePVC, dataPVC, sa, repo, customCAObj)
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
	return mover.Complete(), nil
}

func (m *Mover) ensureCache(ctx context.Context,
	dataPVC *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	// Create a separate vh for the Restic cache volume that's based on the main
	// vh, but override options where necessary.
	cacheConfig := []volumehandler.VHOption{
		// build on the datavolume's configuration
		volumehandler.From(m.vh),
	}

	// Cache capacity defaults to 1Gi but can be overridden
	cacheCapacity := resource.MustParse("1Gi")
	if m.cacheCapacity != nil {
		cacheCapacity = *m.cacheCapacity
	}
	cacheConfig = append(cacheConfig, volumehandler.Capacity(&cacheCapacity))

	// AccessModes are generated in the following priority:
	// 1. Directly specified cache accessMode
	// 2. Directly specified volume accessMode
	// 3. Inherited from the source/data PVC
	if m.cacheAccessModes != nil {
		cacheConfig = append(cacheConfig, volumehandler.AccessModes(m.cacheAccessModes))
	} else if len(m.vh.GetAccessModes()) == 0 {
		cacheConfig = append(cacheConfig, volumehandler.AccessModes(dataPVC.Spec.AccessModes))
	}

	if m.cacheStorageClassName != nil {
		cacheConfig = append(cacheConfig, volumehandler.StorageClassName(m.cacheStorageClassName))
	}

	cacheVh, err := volumehandler.NewVolumeHandler(cacheConfig...)
	if err != nil {
		return nil, err
	}

	// Allocate cache volume
	cacheName := mover.VolSyncPrefix + m.owner.GetName() + "-cache"
	m.logger.Info("allocating cache volume", "PVC", cacheName)
	return cacheVh.EnsureNewPVC(ctx, m.logger, cacheName)
}

func (m *Mover) ensureSourcePVC(ctx context.Context) (*corev1.PersistentVolumeClaim, error) {
	srcPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      *m.mainPVCName,
			Namespace: m.owner.GetNamespace(),
		},
	}
	if err := m.client.Get(ctx, client.ObjectKeyFromObject(srcPVC), srcPVC); err != nil {
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

func (m *Mover) validateRepository(ctx context.Context) (*corev1.Secret, error) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.repositoryName,
			Namespace: m.owner.GetNamespace(),
		},
	}
	logger := m.logger.WithValues("repositorySecret", client.ObjectKeyFromObject(secret))
	if err := utils.GetAndValidateSecret(ctx, m.client, logger, secret,
		"RESTIC_REPOSITORY", "RESTIC_PASSWORD"); err != nil {
		logger.Error(err, "Restic config secret does not contain the proper fields")
		return nil, err
	}
	return secret, nil
}

//nolint:funlen
func (m *Mover) ensureJob(ctx context.Context, cachePVC *corev1.PersistentVolumeClaim,
	dataPVC *corev1.PersistentVolumeClaim, sa *corev1.ServiceAccount, repo *corev1.Secret,
	customCAObj utils.CustomCAObject) (*batchv1.Job, error) {
	dir := "src"
	if !m.isSource {
		dir = "dst"
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mover.VolSyncPrefix + dir + "-" + m.owner.GetName(),
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
		backoffLimit := int32(8)
		job.Spec.BackoffLimit = &backoffLimit
		parallelism := int32(1)
		if m.paused {
			parallelism = int32(0)
		}
		job.Spec.Parallelism = &parallelism
		forgetOptions := generateForgetOptions(m.retainPolicy)
		// set default values
		var restoreAsOf = ""
		var previous = strconv.Itoa(int(int32(0)))

		readOnlyVolume := false
		var actions []string
		if m.isSource {
			actions = []string{"backup"}

			if m.shouldUnlock() {
				// Run restic unlock before backup
				actions = []string{"unlock", "backup"}
			}

			if m.shouldPrune(time.Now()) {
				actions = append(actions, "prune")
			}

			// Set read-only for volume in source mover job spec if the PVC only supports read-only
			readOnlyVolume = utils.PvcIsReadOnly(dataPVC)
		} else {
			actions = []string{"restore"}
			// set the restore selection options when the mover has them
			if m.restoreAsOf != nil {
				restoreAsOf = *m.restoreAsOf
			}
			if m.previous != nil {
				previous = strconv.Itoa(int(*m.previous))
			}
		}
		logger.Info("job actions", "actions", actions)
		podSpec := &job.Spec.Template.Spec

		envVars := []corev1.EnvVar{
			{Name: "FORGET_OPTIONS", Value: forgetOptions},
			{Name: "DATA_DIR", Value: mountPath},
			{Name: "RESTIC_CACHE_DIR", Value: resticCacheMountPath},
			{Name: "RESTORE_AS_OF", Value: restoreAsOf},
			{Name: "SELECT_PREVIOUS", Value: previous},
			// We populate environment variables from the restic repo
			// Secret. They are taken 1-for-1 from the Secret into env vars.
			// The allowed variables are defined by restic.
			// https://restic.readthedocs.io/en/stable/040_backup.html#environment-variables
			// Mandatory variables are needed to define the repository
			// location and its password.
			utils.EnvFromSecret(repo.Name, "RESTIC_REPOSITORY", false),
			utils.EnvFromSecret(repo.Name, "RESTIC_PASSWORD", false),

			// Optional variables
			utils.EnvFromSecret(repo.Name, "RESTIC_COMPRESSION", true), // New in v0.14.0
			utils.EnvFromSecret(repo.Name, "RESTIC_PACK_SIZE", true),   // New in v0.14.0

			utils.EnvFromSecret(repo.Name, "RESTIC_READ_CONCURRENCY", true), // New in v0.15.0

			// Optional variables based on what backend is used for restic
			utils.EnvFromSecret(repo.Name, "AWS_ACCESS_KEY_ID", true),
			utils.EnvFromSecret(repo.Name, "AWS_SECRET_ACCESS_KEY", true),
			utils.EnvFromSecret(repo.Name, "AWS_SESSION_TOKEN", true), // New in v0.14.0
			utils.EnvFromSecret(repo.Name, "AWS_DEFAULT_REGION", true),
			/* These vars are in restic main but not in an official release yet (as of v0.16.3)
			utils.EnvFromSecret(repo.Name, "RESTIC_AWS_ASSUME_ROLE_ARN", true),
			utils.EnvFromSecret(repo.Name, "RESTIC_AWS_ASSUME_ROLE_SESSION_NAME", true),
			utils.EnvFromSecret(repo.Name, "RESTIC_AWS_ASSUME_ROLE_EXTERNAL_ID", true),
			utils.EnvFromSecret(repo.Name, "RESTIC_AWS_ASSUME_ROLE_POLICY", true),
			utils.EnvFromSecret(repo.Name, "RESTIC_AWS_ASSUME_ROLE_REGION", true),
			utils.EnvFromSecret(repo.Name, "RESTIC_AWS_ASSUME_ROLE_STS_ENDPOINT", true),
			*/
			utils.EnvFromSecret(repo.Name, "ST_AUTH", true),
			utils.EnvFromSecret(repo.Name, "ST_USER", true),
			utils.EnvFromSecret(repo.Name, "ST_KEY", true),
			utils.EnvFromSecret(repo.Name, "OS_AUTH_URL", true),
			utils.EnvFromSecret(repo.Name, "OS_REGION_NAME", true),
			utils.EnvFromSecret(repo.Name, "OS_USERNAME", true),
			utils.EnvFromSecret(repo.Name, "OS_USER_ID", true),
			utils.EnvFromSecret(repo.Name, "OS_PASSWORD", true),
			utils.EnvFromSecret(repo.Name, "OS_TENANT_ID", true),
			utils.EnvFromSecret(repo.Name, "OS_TENANT_NAME", true),
			utils.EnvFromSecret(repo.Name, "OS_USER_DOMAIN_NAME", true),
			utils.EnvFromSecret(repo.Name, "OS_USER_DOMAIN_ID", true),
			utils.EnvFromSecret(repo.Name, "OS_PROJECT_NAME", true),
			utils.EnvFromSecret(repo.Name, "OS_PROJECT_DOMAIN_NAME", true),
			utils.EnvFromSecret(repo.Name, "OS_PROJECT_DOMAIN_ID", true),
			utils.EnvFromSecret(repo.Name, "OS_TRUST_ID", true),
			utils.EnvFromSecret(repo.Name, "OS_APPLICATION_CREDENTIAL_ID", true),
			utils.EnvFromSecret(repo.Name, "OS_APPLICATION_CREDENTIAL_NAME", true),
			utils.EnvFromSecret(repo.Name, "OS_APPLICATION_CREDENTIAL_SECRET", true),
			utils.EnvFromSecret(repo.Name, "OS_STORAGE_URL", true),
			utils.EnvFromSecret(repo.Name, "OS_AUTH_TOKEN", true),
			utils.EnvFromSecret(repo.Name, "B2_ACCOUNT_ID", true),
			utils.EnvFromSecret(repo.Name, "B2_ACCOUNT_KEY", true),
			utils.EnvFromSecret(repo.Name, "AZURE_ACCOUNT_NAME", true),
			utils.EnvFromSecret(repo.Name, "AZURE_ACCOUNT_KEY", true),
			utils.EnvFromSecret(repo.Name, "AZURE_ACCOUNT_SAS", true),     // New in v0.14.0
			utils.EnvFromSecret(repo.Name, "AZURE_ENDPOINT_SUFFIX", true), // New in v0.16.0
			utils.EnvFromSecret(repo.Name, "GOOGLE_PROJECT_ID", true),
			utils.EnvFromSecret(repo.Name, "RESTIC_REST_USERNAME", true), // New in v0.16.1
			utils.EnvFromSecret(repo.Name, "RESTIC_REST_PASSWORD", true), // New in v0.16.1
		}

		// Rclone env vars for restic if they are in the secret
		envVars = utils.AppendRCloneEnvVars(repo, envVars)

		// Cluster-wide proxy settings
		envVars = utils.AppendEnvVarsForClusterWideProxy(envVars)

		podSpec.Containers = []corev1.Container{{
			Name:    "restic",
			Env:     envVars,
			Command: []string{"/mover-restic/entry.sh"},
			Args:    actions,
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
				{Name: resticCache, MountPath: resticCacheMountPath},
				{Name: "tempdir", MountPath: "/tmp"},
			},
		}}
		podSpec.RestartPolicy = corev1.RestartPolicyNever
		podSpec.ServiceAccountName = sa.Name
		podSpec.Volumes = []corev1.Volume{
			{Name: dataVolumeName, VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: dataPVC.Name,
					ReadOnly:  readOnlyVolume,
				}},
			},
			{Name: resticCache, VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: cachePVC.Name,
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
			podSpec.NodeSelector = affinity.NodeSelector
			podSpec.Tolerations = affinity.Tolerations
		}
		if customCAObj != nil {
			// Tell mover where to find the cert
			podSpec.Containers[0].Env = append(podSpec.Containers[0].Env, corev1.EnvVar{
				Name:  "CUSTOM_CA",
				Value: path.Join(resticCAMountPath, resticCAFilename),
			})
			// Mount the custom CA certificate
			podSpec.Containers[0].VolumeMounts =
				append(podSpec.Containers[0].VolumeMounts, corev1.VolumeMount{
					Name:      "custom-ca",
					MountPath: resticCAMountPath,
				})
			podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
				Name:         "custom-ca",
				VolumeSource: customCAObj.GetVolumeSource(resticCAFilename),
			})
		}
		// We handle GOOGLE_APPLICATION_CREDENTIALS specially...
		// restic expects it to be an env var pointing to a file w/ the
		// credentials, but we have users provide the actual file data in the
		// Secret under that key name. The following code sets the env var to be
		// what restic expects, then mounts just that Secret key into the
		// container, pointed to by the env var.
		if _, ok := repo.Data["GOOGLE_APPLICATION_CREDENTIALS"]; ok {
			container := &podSpec.Containers[0]
			// Tell restic where to look for the credential file
			container.Env = append(container.Env, corev1.EnvVar{
				Name:  "GOOGLE_APPLICATION_CREDENTIALS",
				Value: path.Join(credentialDir, gcsCredentialFile),
			})
			// Mount the credential file
			container.VolumeMounts =
				append(container.VolumeMounts, corev1.VolumeMount{
					Name:      "gcs-credentials",
					MountPath: credentialDir,
				})
			podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
				Name: "gcs-credentials",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: repo.Name,
						Items: []corev1.KeyToPath{
							{Key: "GOOGLE_APPLICATION_CREDENTIALS", Path: gcsCredentialFile},
						},
					},
				},
			})
		}

		// Update the job securityContext, podLabels and resourceRequirements from moverConfig (if specified)
		utils.UpdatePodTemplateSpecFromMoverConfig(&job.Spec.Template, m.moverConfig, corev1.ResourceRequirements{})

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

	if m.isSource {
		if m.shouldUnlock() {
			// Make sure status matches unlock after successful job
			m.sourceStatus.LastUnlocked = m.unlock
			logger.Info("unlock completed", ".Status.Restic.LastUnlocked", m.sourceStatus.LastUnlocked)
		} else if m.unlock == "" {
			// Unset lastUnlocked in status if unlock is no longer set in the spec
			m.sourceStatus.LastUnlocked = ""
		}

		if m.shouldPrune(time.Now()) {
			now := metav1.Now()
			m.sourceStatus.LastPruned = &now
			logger.Info("prune completed", ".Status.Restic.LastPruned", m.sourceStatus.LastPruned)
		}
	}

	// update status with mover logs from successful job
	utils.UpdateMoverStatusForSuccessfulJob(ctx, m.logger, m.latestMoverStatus, job.GetName(), job.GetNamespace(),
		LogLineFilterSuccess)

	// We only continue reconciling if the restic job has completed
	return job, nil
}

func (m *Mover) shouldPrune(current time.Time) bool {
	delta := time.Hour * 24 * 7 // default prune every 7 days
	if m.pruneInterval != nil {
		delta = time.Hour * 24 * time.Duration(*m.pruneInterval)
	}
	// If we've never pruned, the 1st one should be "delta" after creation.
	lastPruned := m.owner.GetCreationTimestamp().Time
	if !m.sourceStatus.LastPruned.IsZero() {
		lastPruned = m.sourceStatus.LastPruned.Time
	}
	return current.After(lastPruned.Add(delta))
}

func (m *Mover) shouldUnlock() bool {
	if m.unlock != "" && m.sourceStatus.LastUnlocked != m.unlock {
		return true
	}
	return false
}

func generateForgetOptions(policy *volsyncv1alpha1.ResticRetainPolicy) string {
	const defaultForget = "--keep-last 1"

	if policy == nil { // Retain policy isn't present
		return defaultForget
	}

	var forget string
	optionTable := []struct {
		opt   string
		value *int32
	}{
		{"--keep-hourly", policy.Hourly},
		{"--keep-daily", policy.Daily},
		{"--keep-weekly", policy.Weekly},
		{"--keep-monthly", policy.Monthly},
		{"--keep-yearly", policy.Yearly},
	}
	for _, v := range optionTable {
		if v.value != nil {
			forget += fmt.Sprintf(" %s %d", v.opt, *v.value)
		}
	}
	if policy.Within != nil {
		forget += fmt.Sprintf(" --keep-within %s", *policy.Within)
	}
	if policy.Last != nil {
		forget += fmt.Sprintf(" --keep-last %s", *policy.Last)
	}

	if len(forget) == 0 { // Retain policy was present, but empty
		return defaultForget
	}
	return forget
}
