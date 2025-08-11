//go:build !disable_kopia

/*
Copyright 2024 The VolSync authors.

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

package kopia

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/prometheus/client_golang/prometheus"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	vserrors "github.com/backube/volsync/internal/controller/errors"
	"github.com/backube/volsync/internal/controller/mover"
	"github.com/backube/volsync/internal/controller/utils"
	"github.com/backube/volsync/internal/controller/volumehandler"
)

const (
	kopiaCacheMountPath     = "/cache"
	sourceMountPath         = "/data"
	destinationMountPath    = "/restore/data"
	dataVolumeName          = "data"
	kopiaCache              = "cache"
	restoreVolumeName       = "restore"
	kopiaCAMountPath        = "/customCA"
	kopiaCAFilename         = "ca.crt"
	credentialDir           = "/credentials"
	repositoryPVCVolName    = "repository-pvc"
	repositoryPVCMountPath  = "/kopia"
	repositoryPath          = "/kopia/repository"
	gcsCredentialFile       = "gcs.json"
	kopiaPolicyMountPath    = "/kopia-config"
	defaultGlobalPolicyFile = "global-policy.json"
	defaultRepoConfigFile   = "repository.config"
	operationBackup         = "backup"
)

// Mover is the reconciliation logic for the Kopia-based data mover.
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
	policyConfig          *volsyncv1alpha1.KopiaPolicySpec
	privileged            bool
	latestMoverStatus     *volsyncv1alpha1.MoverStatus
	moverConfig           volsyncv1alpha1.MoverConfig
	metrics               kopiaMetrics
	// User identity for multi-tenancy
	username string
	hostname string
	// Source-only fields
	sourcePathOverride  *string
	maintenanceInterval *int32
	retainPolicy        *volsyncv1alpha1.KopiaRetainPolicy
	compression         string
	parallelism         *int32
	actions             *volsyncv1alpha1.KopiaActions
	sourceStatus        *volsyncv1alpha1.ReplicationSourceKopiaStatus
	repositoryPVC       *string
	// Destination-only fields
	restoreAsOf       *string
	shallow           *int32
	previous          *int32
	cleanupTempPVC    bool
	cleanupCachePVC   bool
	destinationStatus *volsyncv1alpha1.ReplicationDestinationKopiaStatus
}

var _ mover.Mover = &Mover{}

// All object types that are temporary/per-iteration should be listed here. The
// individual objects to be cleaned up must also be marked.
var cleanupTypes = []client.Object{
	&corev1.PersistentVolumeClaim{},
	&snapv1.VolumeSnapshot{},
	&batchv1.Job{},
}

func (m *Mover) Name() string { return kopiaMoverName }

func (m *Mover) Synchronize(ctx context.Context) (mover.Result, error) {
	if m.paused {
		return mover.Complete(), nil
	}

	// Record operation start time for metrics
	operationStart := time.Now()
	operation := operationBackup
	if !m.isSource {
		operation = "restore"
	}

	// Track repository connectivity
	labels := m.getMetricLabels("")
	m.metrics.RepositoryConnectivity.With(labels).Set(1) // Assume connected initially

	// Setup prerequisites
	dataPVC, cachePVC, sa, repo, customCAObj, policyConfigObj, err := m.setupPrerequisites(ctx)
	if err != nil {
		// Repository connectivity issue or configuration error
		m.metrics.RepositoryConnectivity.With(labels).Set(0)
		m.recordOperationFailure(operation, "prerequisites_failed")
		return mover.InProgress(), err
	}

	// Start and monitor job
	job, err := m.ensureJob(ctx, cachePVC, dataPVC, sa, repo, customCAObj, policyConfigObj)
	if job == nil || err != nil {
		m.recordOperationFailure(operation, "job_creation_failed")
		return mover.InProgress(), err
	}

	// Handle job completion
	result, err := m.handleJobCompletion(ctx, job, dataPVC)

	// Record metrics based on job completion
	if err != nil {
		m.recordOperationFailure(operation, "job_execution_failed")
	} else if result.Completed {
		// Job completed successfully
		duration := time.Since(operationStart)
		m.recordOperationSuccess(operation, duration)

		// Record maintenance if applicable
		if m.isSource && m.shouldRunMaintenance() {
			m.recordMaintenanceOperation()
		}
	}

	return result, err
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
	return pvc, err
}

func (m *Mover) ensureDestinationPVC(ctx context.Context) (*corev1.PersistentVolumeClaim, error) {
	isProvidedPVC, dataPVCName := m.getDestinationPVCName()
	if isProvidedPVC {
		return m.vh.UseProvidedPVC(ctx, dataPVCName)
	}
	// Need to allocate the incoming data volume
	return m.vh.EnsureNewPVC(ctx, m.logger, dataPVCName, m.cleanupTempPVC)
}

func (m *Mover) getDestinationPVCName() (bool, string) {
	if m.mainPVCName == nil {
		// Creating new PVC
		newPvcName := mover.VolSyncPrefix + m.owner.GetName() + "-dest"
		return false, newPvcName
	}
	return true, *m.mainPVCName
}

func (m *Mover) ensureCache(ctx context.Context,
	dataPVC *corev1.PersistentVolumeClaim, isTemporary bool) (*corev1.PersistentVolumeClaim, error) {
	// Determine cache configuration strategy:
	// 1. Pure fallback: No configuration at all -> EmptyDir with default size limit
	// 2. Capacity-only: Only cacheCapacity specified -> EmptyDir with specified size limit
	// 3. Full PVC: StorageClass or AccessModes specified -> PVC

	hasPVCConfig := m.cacheStorageClassName != nil || m.cacheAccessModes != nil
	hasCapacityOnly := m.cacheCapacity != nil && !hasPVCConfig
	hasNoCacheConfig := m.cacheCapacity == nil && !hasPVCConfig

	// Use EmptyDir fallback for scenarios 1 and 2
	if hasNoCacheConfig || hasCapacityOnly {
		if hasNoCacheConfig {
			m.logger.V(1).Info("No cache configuration specified, will use EmptyDir volume with default size limit")
		} else {
			m.logger.V(1).Info("Cache capacity only specified, will use EmptyDir volume with size limit",
				"capacity", m.cacheCapacity.String())
		}
		return nil, nil
	}

	// Create a separate vh for the Kopia cache volume that's based on the main
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
	dir := "src"
	if !m.isSource {
		dir = "dst"
	}
	cacheName := mover.VolSyncPrefix + dir + "-" + m.owner.GetName() + "-cache"
	m.logger.Info("allocating cache volume", "PVC", cacheName, "isTemporary", isTemporary)
	return cacheVh.EnsureNewPVC(ctx, m.logger, cacheName, isTemporary)
}

func (m *Mover) validateRepository(ctx context.Context) (*corev1.Secret, error) {
	// If using repository PVC, validate the PVC first
	if m.repositoryPVC != nil {
		if err := m.validateRepositoryPVC(ctx); err != nil {
			return nil, err
		}
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.repositoryName,
			Namespace: m.owner.GetNamespace(),
		},
	}
	logger := m.logger.WithValues("repositorySecret", client.ObjectKeyFromObject(secret))
	if err := utils.GetAndValidateSecret(ctx, m.client, logger, secret,
		"KOPIA_REPOSITORY", "KOPIA_PASSWORD"); err != nil {
		logger.Error(err, "Kopia config secret does not contain the proper fields")
		return nil, err
	}
	return secret, nil
}

// validateRepositoryPVC validates that the repository PVC exists and is bound
func (m *Mover) validateRepositoryPVC(ctx context.Context) error {
	if m.repositoryPVC == nil {
		return nil
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      *m.repositoryPVC,
			Namespace: m.owner.GetNamespace(),
		},
	}

	if err := m.client.Get(ctx, client.ObjectKeyFromObject(pvc), pvc); err != nil {
		m.logger.Error(err, "Failed to get repository PVC",
			"pvc", *m.repositoryPVC)
		return fmt.Errorf("repository PVC %s not found: %w",
			*m.repositoryPVC, err)
	}

	// Check if PVC is bound
	if pvc.Status.Phase != corev1.ClaimBound {
		m.logger.Info("Repository PVC is not bound yet",
			"pvc", *m.repositoryPVC,
			"phase", pvc.Status.Phase)
		return fmt.Errorf("repository PVC %s is not bound (phase: %s)",
			*m.repositoryPVC, pvc.Status.Phase)
	}

	m.logger.V(1).Info("Repository PVC validated successfully",
		"pvc", *m.repositoryPVC)
	return nil
}

func (m *Mover) validatePolicyConfig(ctx context.Context) (utils.CustomCAObject, error) {
	if m.policyConfig == nil {
		return nil, nil
	}

	// Validate JSON format if repositoryConfig is specified
	if m.policyConfig.RepositoryConfig != nil && *m.policyConfig.RepositoryConfig != "" {
		if !json.Valid([]byte(*m.policyConfig.RepositoryConfig)) {
			return nil, fmt.Errorf("invalid JSON in repositoryConfig")
		}
		m.logger.V(1).Info("JSON validation passed for repositoryConfig")
	}

	// Validate file-based configuration if specified
	if m.policyConfig.SecretName != "" || m.policyConfig.ConfigMapName != "" {
		// Convert KopiaPolicySpec to CustomCASpec for reuse of validation logic
		policyCASpec := volsyncv1alpha1.CustomCASpec{
			SecretName:    m.policyConfig.SecretName,
			ConfigMapName: m.policyConfig.ConfigMapName,
			// We don't need to specify a Key since we'll mount the entire ConfigMap/Secret
		}

		return utils.ValidateCustomCA(ctx, m.client, m.logger,
			m.owner.GetNamespace(), policyCASpec)
	}

	return nil, nil
}

func (m *Mover) ensureJob(ctx context.Context, cachePVC *corev1.PersistentVolumeClaim,
	dataPVC *corev1.PersistentVolumeClaim, sa *corev1.ServiceAccount, repo *corev1.Secret,
	customCAObj utils.CustomCAObject, policyConfigObj utils.CustomCAObject) (*batchv1.Job, error) {
	dir := "src"
	if !m.isSource {
		dir = "dst"
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.GetJobName(mover.VolSyncPrefix+dir+"-", m.owner),
			Namespace: m.owner.GetNamespace(),
		},
	}
	logger := m.logger.WithValues("job", client.ObjectKeyFromObject(job))

	_, err := utils.CreateOrUpdateDeleteOnImmutableErr(ctx, m.client, job, logger, func() error {
		return m.configureJobSpec(ctx, job, cachePVC, dataPVC, sa, repo, customCAObj, policyConfigObj, logger)
	})
	return m.handleJobStatus(ctx, job, logger, err)
}

// configureJobSpec configures the job specification with all required settings
func (m *Mover) configureJobSpec(ctx context.Context, job *batchv1.Job, cachePVC *corev1.PersistentVolumeClaim,
	dataPVC *corev1.PersistentVolumeClaim, sa *corev1.ServiceAccount, repo *corev1.Secret,
	customCAObj utils.CustomCAObject, policyConfigObj utils.CustomCAObject, logger logr.Logger) error {
	if err := ctrl.SetControllerReference(m.owner, job, m.client.Scheme()); err != nil {
		logger.Error(err, utils.ErrUnableToSetControllerRef)
		return err
	}

	m.setupJobMetadata(job)
	readOnlyVolume, actions := m.determineJobActions(dataPVC)
	logger.Info("job actions", "actions", actions)

	podSpec := &job.Spec.Template.Spec
	envVars := m.buildEnvironmentVariables(repo)
	m.configureContainer(podSpec, envVars, actions, readOnlyVolume, sa)
	m.configureBasicVolumes(podSpec, dataPVC, readOnlyVolume)
	m.configureCacheVolume(podSpec, cachePVC)

	if err := m.configureAffinity(ctx, podSpec, dataPVC, logger); err != nil {
		return err
	}

	m.configureCustomCA(podSpec, customCAObj)
	m.configurePolicyConfig(podSpec, policyConfigObj)
	m.configureCredentials(podSpec, repo)

	// Configure repository PVC if specified
	if err := m.configureRepositoryPVC(podSpec); err != nil {
		logger.Error(err, "failed to configure repository PVC")
		return err
	}

	// Update the job securityContext, podLabels and resourceRequirements from moverConfig (if specified)
	utils.UpdatePodTemplateSpecFromMoverConfig(&job.Spec.Template, m.moverConfig, corev1.ResourceRequirements{})

	m.configureSecurityContext(podSpec)
	return nil
}

// setupJobMetadata sets up basic job metadata and configuration
func (m *Mover) setupJobMetadata(job *batchv1.Job) {
	utils.SetOwnedByVolSync(job)
	utils.MarkForCleanup(m.owner, job)
	job.Spec.Template.Name = job.Name
	utils.SetOwnedByVolSync(&job.Spec.Template)
	backoffLimit := int32(8)
	job.Spec.BackoffLimit = &backoffLimit
	parallelism := int32(1)
	if m.paused {
		parallelism = int32(0)
	}
	job.Spec.Parallelism = &parallelism
}

// determineJobActions determines job actions and volume access mode
func (m *Mover) determineJobActions(dataPVC *corev1.PersistentVolumeClaim) (bool, []string) {
	readOnlyVolume := false
	var actions []string
	if m.isSource {
		actions = []string{operationBackup}
		// Set read-only for volume in source mover job spec if the PVC only supports read-only
		readOnlyVolume = utils.PvcIsReadOnly(dataPVC)
	} else {
		actions = []string{"restore"}
	}
	return readOnlyVolume, actions
}

// buildEnvironmentVariables creates the base environment variables for the container
func (m *Mover) buildEnvironmentVariables(repo *corev1.Secret) []corev1.EnvVar {
	envVars := m.buildBasicEnvironmentVariables()
	envVars = append(envVars, m.buildRepositoryEnvironmentVariables(repo)...)
	envVars = append(envVars, m.buildBackendEnvironmentVariables(repo)...)
	envVars = m.addSourceDestinationEnvVars(envVars)
	envVars = utils.AppendEnvVarsForClusterWideProxy(envVars)
	envVars = m.addIdentityEnvironmentVariables(envVars)
	envVars = utils.AppendDebugMoverEnvVar(m.owner, envVars)
	return envVars
}

// buildBasicEnvironmentVariables creates basic environment variables
func (m *Mover) buildBasicEnvironmentVariables() []corev1.EnvVar {
	// Use different mount paths for source (backup) vs destination (restore)
	dataDir := sourceMountPath
	if !m.isSource {
		dataDir = destinationMountPath
	}
	return []corev1.EnvVar{
		{Name: "DATA_DIR", Value: dataDir},
		{Name: "KOPIA_CACHE_DIR", Value: kopiaCacheMountPath},
	}
}

// buildRepositoryEnvironmentVariables creates repository-related environment variables
func (m *Mover) buildRepositoryEnvironmentVariables(repo *corev1.Secret) []corev1.EnvVar {
	envVars := []corev1.EnvVar{
		// Mandatory variables are needed to define the repository location and its password
		utils.EnvFromSecret(repo.Name, "KOPIA_PASSWORD", false),
		// Optional manual configuration for advanced repository settings
		utils.EnvFromSecret(repo.Name, "KOPIA_MANUAL_CONFIG", true),
	}

	// If using repository PVC, set KOPIA_REPOSITORY to filesystem:// URL
	// Otherwise, get it from the secret
	if m.repositoryPVC != nil {
		// Use filesystem:// URL for PVC-based repositories
		envVars = append(envVars, corev1.EnvVar{
			Name:  "KOPIA_REPOSITORY",
			Value: "filesystem://" + repositoryPath,
		})
	} else {
		// Get repository URL from secret for other backends
		envVars = append(envVars, utils.EnvFromSecret(repo.Name, "KOPIA_REPOSITORY", false))
	}

	return envVars
}

// buildBackendEnvironmentVariables creates backend-specific environment variables
func (m *Mover) buildBackendEnvironmentVariables(repo *corev1.Secret) []corev1.EnvVar {
	envVars := m.buildAWSEnvironmentVariables(repo)
	envVars = append(envVars, m.buildAzureEnvironmentVariables(repo)...)
	envVars = append(envVars, m.buildGoogleEnvironmentVariables(repo)...)
	envVars = append(envVars, m.buildB2EnvironmentVariables(repo)...)
	envVars = append(envVars, m.buildWebDAVEnvironmentVariables(repo)...)
	envVars = append(envVars, m.buildSFTPEnvironmentVariables(repo)...)
	envVars = append(envVars, m.buildRcloneEnvironmentVariables(repo)...)
	return envVars
}

// buildAWSEnvironmentVariables creates AWS S3 backend environment variables
func (m *Mover) buildAWSEnvironmentVariables(repo *corev1.Secret) []corev1.EnvVar {
	return []corev1.EnvVar{
		// AWS standard variables
		utils.EnvFromSecret(repo.Name, "AWS_ACCESS_KEY_ID", true),
		utils.EnvFromSecret(repo.Name, "AWS_SECRET_ACCESS_KEY", true),
		utils.EnvFromSecret(repo.Name, "AWS_SESSION_TOKEN", true),
		utils.EnvFromSecret(repo.Name, "AWS_DEFAULT_REGION", true),
		utils.EnvFromSecret(repo.Name, "AWS_REGION", true),
		utils.EnvFromSecret(repo.Name, "AWS_PROFILE", true),
		utils.EnvFromSecret(repo.Name, "AWS_S3_ENDPOINT", true),
		// Kopia-specific S3 variables (support both naming conventions)
		utils.EnvFromSecret(repo.Name, "KOPIA_S3_BUCKET", true),
		utils.EnvFromSecret(repo.Name, "KOPIA_S3_ENDPOINT", true),
		utils.EnvFromSecret(repo.Name, "KOPIA_S3_DISABLE_TLS", true),
		utils.EnvFromSecret(repo.Name, "AWS_S3_DISABLE_TLS", true), // Support AWS prefix too
	}
}

// buildAzureEnvironmentVariables creates Azure backend environment variables
func (m *Mover) buildAzureEnvironmentVariables(repo *corev1.Secret) []corev1.EnvVar {
	return []corev1.EnvVar{
		// Azure standard variables
		utils.EnvFromSecret(repo.Name, "AZURE_ACCOUNT_NAME", true),
		utils.EnvFromSecret(repo.Name, "AZURE_ACCOUNT_KEY", true),
		utils.EnvFromSecret(repo.Name, "AZURE_ACCOUNT_SAS", true),
		utils.EnvFromSecret(repo.Name, "AZURE_ENDPOINT_SUFFIX", true),
		utils.EnvFromSecret(repo.Name, "AZURE_STORAGE_ACCOUNT", true),
		utils.EnvFromSecret(repo.Name, "AZURE_STORAGE_KEY", true),
		utils.EnvFromSecret(repo.Name, "AZURE_STORAGE_SAS_TOKEN", true),
		// Kopia-specific Azure variables (support both naming conventions)
		utils.EnvFromSecret(repo.Name, "KOPIA_AZURE_CONTAINER", true),
		utils.EnvFromSecret(repo.Name, "KOPIA_AZURE_STORAGE_ACCOUNT", true),
		utils.EnvFromSecret(repo.Name, "KOPIA_AZURE_STORAGE_KEY", true),
	}
}

// buildGoogleEnvironmentVariables creates Google Cloud backend environment variables
func (m *Mover) buildGoogleEnvironmentVariables(repo *corev1.Secret) []corev1.EnvVar {
	return []corev1.EnvVar{
		// Google Cloud standard variables
		utils.EnvFromSecret(repo.Name, "GOOGLE_PROJECT_ID", true),
		utils.EnvFromSecret(repo.Name, "GOOGLE_APPLICATION_CREDENTIALS", true),
		utils.EnvFromSecret(repo.Name, "GOOGLE_DRIVE_FOLDER_ID", true),
		utils.EnvFromSecret(repo.Name, "GOOGLE_DRIVE_CREDENTIALS", true),
		// Kopia-specific GCS variables (support both naming conventions)
		utils.EnvFromSecret(repo.Name, "KOPIA_GCS_BUCKET", true),
		utils.EnvFromSecret(repo.Name, "GCS_BUCKET", true), // Support standard GCS prefix
	}
}

// buildB2EnvironmentVariables creates Backblaze B2 backend environment variables
func (m *Mover) buildB2EnvironmentVariables(repo *corev1.Secret) []corev1.EnvVar {
	return []corev1.EnvVar{
		utils.EnvFromSecret(repo.Name, "B2_ACCOUNT_ID", true),
		utils.EnvFromSecret(repo.Name, "B2_APPLICATION_KEY", true),
		utils.EnvFromSecret(repo.Name, "KOPIA_B2_BUCKET", true),
	}
}

// buildWebDAVEnvironmentVariables creates WebDAV backend environment variables
func (m *Mover) buildWebDAVEnvironmentVariables(repo *corev1.Secret) []corev1.EnvVar {
	return []corev1.EnvVar{
		utils.EnvFromSecret(repo.Name, "WEBDAV_URL", true),
		utils.EnvFromSecret(repo.Name, "WEBDAV_USERNAME", true),
		utils.EnvFromSecret(repo.Name, "WEBDAV_PASSWORD", true),
	}
}

// buildSFTPEnvironmentVariables creates SFTP backend environment variables
func (m *Mover) buildSFTPEnvironmentVariables(repo *corev1.Secret) []corev1.EnvVar {
	return []corev1.EnvVar{
		utils.EnvFromSecret(repo.Name, "SFTP_HOST", true),
		utils.EnvFromSecret(repo.Name, "SFTP_PORT", true),
		utils.EnvFromSecret(repo.Name, "SFTP_USERNAME", true),
		utils.EnvFromSecret(repo.Name, "SFTP_PASSWORD", true),
		utils.EnvFromSecret(repo.Name, "SFTP_PATH", true),
		utils.EnvFromSecret(repo.Name, "SFTP_KEY_FILE", true),
	}
}

// buildRcloneEnvironmentVariables creates Rclone backend environment variables
func (m *Mover) buildRcloneEnvironmentVariables(repo *corev1.Secret) []corev1.EnvVar {
	return []corev1.EnvVar{
		utils.EnvFromSecret(repo.Name, "RCLONE_REMOTE_PATH", true),
		utils.EnvFromSecret(repo.Name, "RCLONE_EXE", true),
		utils.EnvFromSecret(repo.Name, "RCLONE_CONFIG", true),
	}
}

// addIdentityEnvironmentVariables adds username and hostname overrides for multi-tenancy
func (m *Mover) addIdentityEnvironmentVariables(envVars []corev1.EnvVar) []corev1.EnvVar {
	// Set the requested identity in status for destinations
	if !m.isSource && m.destinationStatus != nil {
		m.destinationStatus.RequestedIdentity = fmt.Sprintf("%s@%s", m.username, m.hostname)
	}

	envVars = append(envVars,
		corev1.EnvVar{
			Name:  "KOPIA_OVERRIDE_USERNAME",
			Value: m.username,
		},
		corev1.EnvVar{
			Name:  "KOPIA_OVERRIDE_HOSTNAME",
			Value: m.hostname,
		},
	)

	// Add discovery flag for destinations
	if !m.isSource {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "KOPIA_DISCOVER_SNAPSHOTS",
			Value: "true",
		})
	}

	return envVars
}

// addSourceDestinationEnvVars adds environment variables specific to source or destination
func (m *Mover) addSourceDestinationEnvVars(envVars []corev1.EnvVar) []corev1.EnvVar {
	if m.isSource {
		return m.addSourceEnvVars(envVars)
	}
	return m.addDestinationEnvVars(envVars)
}

// addSourceEnvVars adds source-specific environment variables
func (m *Mover) addSourceEnvVars(envVars []corev1.EnvVar) []corev1.EnvVar {
	if m.compression != "" {
		envVars = append(envVars, corev1.EnvVar{Name: "KOPIA_COMPRESSION", Value: m.compression})
	}
	if m.parallelism != nil {
		envVars = append(envVars, corev1.EnvVar{Name: "KOPIA_PARALLELISM", Value: strconv.Itoa(int(*m.parallelism))})
	}
	if m.sourcePathOverride != nil {
		envVars = append(envVars, corev1.EnvVar{Name: "KOPIA_SOURCE_PATH_OVERRIDE", Value: *m.sourcePathOverride})
	}

	// Repository path is now set in KOPIA_REPOSITORY as filesystem:// URL
	// when using repository PVC - handled in buildRepositoryEnvironmentVariables

	// Add retention policy
	envVars = m.addRetentionPolicyEnvVars(envVars)

	// Add actions
	envVars = m.addActionsEnvVars(envVars)

	return envVars
}

// addDestinationEnvVars adds destination-specific environment variables
func (m *Mover) addDestinationEnvVars(envVars []corev1.EnvVar) []corev1.EnvVar {
	if m.restoreAsOf != nil {
		envVars = append(envVars, corev1.EnvVar{Name: "KOPIA_RESTORE_AS_OF", Value: *m.restoreAsOf})
	}
	if m.shallow != nil {
		envVars = append(envVars, corev1.EnvVar{Name: "KOPIA_SHALLOW", Value: strconv.Itoa(int(*m.shallow))})
	}
	if m.previous != nil {
		envVars = append(envVars, corev1.EnvVar{Name: "KOPIA_PREVIOUS", Value: strconv.Itoa(int(*m.previous))})
	}
	// Pass sourcePathOverride to destination jobs for correct snapshot path restoration
	if m.sourcePathOverride != nil {
		envVars = append(envVars, corev1.EnvVar{Name: "KOPIA_SOURCE_PATH_OVERRIDE", Value: *m.sourcePathOverride})
	}
	return envVars
}

// addRetentionPolicyEnvVars adds retention policy environment variables
func (m *Mover) addRetentionPolicyEnvVars(envVars []corev1.EnvVar) []corev1.EnvVar {
	if m.retainPolicy == nil {
		return envVars
	}

	if m.retainPolicy.Hourly != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name: "KOPIA_RETAIN_HOURLY", Value: strconv.Itoa(int(*m.retainPolicy.Hourly))})
	}
	if m.retainPolicy.Daily != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name: "KOPIA_RETAIN_DAILY", Value: strconv.Itoa(int(*m.retainPolicy.Daily))})
	}
	if m.retainPolicy.Weekly != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name: "KOPIA_RETAIN_WEEKLY", Value: strconv.Itoa(int(*m.retainPolicy.Weekly))})
	}
	if m.retainPolicy.Monthly != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name: "KOPIA_RETAIN_MONTHLY", Value: strconv.Itoa(int(*m.retainPolicy.Monthly))})
	}
	if m.retainPolicy.Yearly != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name: "KOPIA_RETAIN_YEARLY", Value: strconv.Itoa(int(*m.retainPolicy.Yearly))})
	}
	return envVars
}

// addActionsEnvVars adds action environment variables
func (m *Mover) addActionsEnvVars(envVars []corev1.EnvVar) []corev1.EnvVar {
	if m.actions == nil {
		return envVars
	}

	if m.actions.BeforeSnapshot != "" {
		envVars = append(envVars, corev1.EnvVar{Name: "KOPIA_BEFORE_SNAPSHOT", Value: m.actions.BeforeSnapshot})
	}
	if m.actions.AfterSnapshot != "" {
		envVars = append(envVars, corev1.EnvVar{Name: "KOPIA_AFTER_SNAPSHOT", Value: m.actions.AfterSnapshot})
	}
	return envVars
}

// configureContainer sets up the main container configuration
func (m *Mover) configureContainer(podSpec *corev1.PodSpec, envVars []corev1.EnvVar,
	actions []string, readOnlyVolume bool, sa *corev1.ServiceAccount) {
	// Use different mount paths for source (backup) vs destination (restore)
	dataMountPath := sourceMountPath
	if !m.isSource {
		dataMountPath = destinationMountPath
	}
	volumeMounts := []corev1.VolumeMount{
		{Name: dataVolumeName, MountPath: dataMountPath, ReadOnly: readOnlyVolume},
		{Name: "tempdir", MountPath: "/tmp"},
	}

	// For destination movers, add the /restore volume mount
	// This is needed for Kopia's atomic file operations which create temp files
	// at the parent directory of the restore path
	if !m.isSource {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      restoreVolumeName,
			MountPath: "/restore",
		})
	}

	podSpec.Containers = []corev1.Container{{
		Name:    "kopia",
		Env:     envVars,
		Command: []string{"/mover-kopia/entry.sh"},
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
		VolumeMounts: volumeMounts,
	}}
	podSpec.RestartPolicy = corev1.RestartPolicyNever
	podSpec.ServiceAccountName = sa.Name
}

// configureBasicVolumes sets up basic volumes for the pod
func (m *Mover) configureBasicVolumes(podSpec *corev1.PodSpec,
	dataPVC *corev1.PersistentVolumeClaim, readOnlyVolume bool) {
	volumes := []corev1.Volume{
		{Name: dataVolumeName, VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: dataPVC.Name,
				ReadOnly:  readOnlyVolume,
			}},
		},
		{Name: "tempdir", VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{
				Medium: corev1.StorageMediumMemory,
			}},
		},
	}

	// For destination movers, add the restore volume to make /restore writable
	// This is needed for Kopia's atomic file operations which create temp files
	// at the parent directory of the restore path
	if !m.isSource {
		volumes = append(volumes, corev1.Volume{
			Name: restoreVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}

	podSpec.Volumes = volumes
}

// configureCacheVolume adds cache volume - either PVC or EmptyDir fallback
func (m *Mover) configureCacheVolume(podSpec *corev1.PodSpec, cachePVC *corev1.PersistentVolumeClaim) {
	// Always add the volume mount for cache
	podSpec.Containers[0].VolumeMounts = append(podSpec.Containers[0].VolumeMounts, corev1.VolumeMount{
		Name:      kopiaCache,
		MountPath: kopiaCacheMountPath,
	})

	var cacheVolume corev1.Volume
	if cachePVC != nil {
		// Use PVC when cache is configured
		m.logger.V(1).Info("Using PVC for Kopia cache", "pvc", cachePVC.Name)
		cacheVolume = corev1.Volume{
			Name: kopiaCache,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: cachePVC.Name,
				},
			},
		}
	} else {
		// Use EmptyDir as fallback - this handles both no-config and capacity-only scenarios
		m.logger.V(1).Info("Using EmptyDir for Kopia cache")
		emptyDirSource := &corev1.EmptyDirVolumeSource{}

		// Set size limit based on configuration
		if m.cacheCapacity != nil {
			// User specified capacity - use it for EmptyDir size limit
			m.logger.V(1).Info("Setting EmptyDir size limit from user configuration", "capacity", m.cacheCapacity.String())
			emptyDirSource.SizeLimit = m.cacheCapacity
		} else {
			// No capacity specified - set a reasonable default to prevent unbounded usage
			defaultLimit := resource.MustParse("8Gi")
			m.logger.V(1).Info("Setting default EmptyDir size limit to prevent unbounded usage",
				"defaultCapacity", defaultLimit.String())
			emptyDirSource.SizeLimit = &defaultLimit
		}

		cacheVolume = corev1.Volume{
			Name: kopiaCache,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: emptyDirSource,
			},
		}
	}

	podSpec.Volumes = append(podSpec.Volumes, cacheVolume)
}

// configureAffinity sets up node affinity if using direct copy method
func (m *Mover) configureAffinity(ctx context.Context, podSpec *corev1.PodSpec,
	dataPVC *corev1.PersistentVolumeClaim, logger logr.Logger) error {
	if !m.vh.IsCopyMethodDirect() {
		return nil
	}

	affinity, err := utils.AffinityFromVolume(ctx, m.client, logger, dataPVC)
	if err != nil {
		logger.Error(err, "unable to determine proper affinity", "PVC", client.ObjectKeyFromObject(dataPVC))
		return err
	}
	podSpec.NodeSelector = affinity.NodeSelector
	podSpec.Tolerations = affinity.Tolerations
	return nil
}

// configureCustomCA sets up custom CA configuration if specified
func (m *Mover) configureCustomCA(podSpec *corev1.PodSpec, customCAObj utils.CustomCAObject) {
	if customCAObj == nil {
		return
	}

	// Tell mover where to find the cert
	podSpec.Containers[0].Env = append(podSpec.Containers[0].Env, corev1.EnvVar{
		Name:  "CUSTOM_CA",
		Value: path.Join(kopiaCAMountPath, kopiaCAFilename),
	})
	// Mount the custom CA certificate
	podSpec.Containers[0].VolumeMounts =
		append(podSpec.Containers[0].VolumeMounts, corev1.VolumeMount{
			Name:      "custom-ca",
			MountPath: kopiaCAMountPath,
		})
	podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
		Name:         "custom-ca",
		VolumeSource: customCAObj.GetVolumeSource(kopiaCAFilename),
	})
}

// configurePolicyConfig sets up policy configuration if specified
func (m *Mover) configurePolicyConfig(podSpec *corev1.PodSpec, policyConfigObj utils.CustomCAObject) {
	// Handle structured repository configuration if specified
	if m.policyConfig != nil && m.policyConfig.RepositoryConfig != nil {
		m.configureStructuredRepositoryConfig(podSpec)
	}

	// Handle file-based policy configuration if specified
	if policyConfigObj != nil {
		m.configureFilePolicyConfig(podSpec, policyConfigObj)
	}
}

// configureStructuredRepositoryConfig handles structured repository configuration
func (m *Mover) configureStructuredRepositoryConfig(podSpec *corev1.PodSpec) {
	// Pass the JSON string directly to the environment variable
	if m.policyConfig.RepositoryConfig != nil && *m.policyConfig.RepositoryConfig != "" {
		podSpec.Containers[0].Env = append(podSpec.Containers[0].Env,
			corev1.EnvVar{
				Name:  "KOPIA_STRUCTURED_REPOSITORY_CONFIG",
				Value: *m.policyConfig.RepositoryConfig,
			},
		)
		m.logger.V(1).Info("Added structured repository configuration to pod spec")
	}
}

// configureFilePolicyConfig handles file-based policy configuration
func (m *Mover) configureFilePolicyConfig(podSpec *corev1.PodSpec, policyConfigObj utils.CustomCAObject) {
	// Add environment variables to tell the mover where to find policy files
	podSpec.Containers[0].Env = append(podSpec.Containers[0].Env,
		corev1.EnvVar{
			Name:  "KOPIA_CONFIG_PATH",
			Value: kopiaPolicyMountPath,
		},
	)

	// Add filenames if specified, otherwise use defaults
	globalPolicyFile := defaultGlobalPolicyFile
	if m.policyConfig != nil && m.policyConfig.GlobalPolicyFilename != "" {
		globalPolicyFile = m.policyConfig.GlobalPolicyFilename
	}
	repoConfigFile := defaultRepoConfigFile
	if m.policyConfig != nil && m.policyConfig.RepositoryConfigFilename != "" {
		repoConfigFile = m.policyConfig.RepositoryConfigFilename
	}

	podSpec.Containers[0].Env = append(podSpec.Containers[0].Env,
		corev1.EnvVar{
			Name:  "KOPIA_GLOBAL_POLICY_FILE",
			Value: path.Join(kopiaPolicyMountPath, globalPolicyFile),
		},
		corev1.EnvVar{
			Name:  "KOPIA_REPOSITORY_CONFIG_FILE",
			Value: path.Join(kopiaPolicyMountPath, repoConfigFile),
		},
	)

	// Mount the policy configuration files volume
	podSpec.Containers[0].VolumeMounts =
		append(podSpec.Containers[0].VolumeMounts, corev1.VolumeMount{
			Name:      "kopia-config",
			MountPath: kopiaPolicyMountPath,
		})
	podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
		Name:         "kopia-config",
		VolumeSource: policyConfigObj.GetVolumeSource(""),
	})
}

// configureRepositoryPVC configures the pod to use a filesystem-based repository
// backed by a user-provided PVC. The PVC is mounted at /kopia and
// the repository is created at /kopia/repository for security and isolation.
func (m *Mover) configureRepositoryPVC(podSpec *corev1.PodSpec) error {
	if m.repositoryPVC == nil {
		return nil
	}

	// Check for mount path conflicts
	for _, vm := range podSpec.Containers[0].VolumeMounts {
		if vm.MountPath == repositoryPVCMountPath {
			m.logger.Error(nil, "Mount path conflict detected for repository PVC",
				"mountPath", repositoryPVCMountPath,
				"existingVolume", vm.Name)
			return fmt.Errorf("mount path %s is already in use by volume %s", repositoryPVCMountPath, vm.Name)
		}
	}

	volumeName := repositoryPVCVolName

	// Add volume mount for the repository PVC
	podSpec.Containers[0].VolumeMounts = append(podSpec.Containers[0].VolumeMounts, corev1.VolumeMount{
		Name:      volumeName,
		MountPath: repositoryPVCMountPath,
		ReadOnly:  false, // Repository needs write access
	})

	// Add volume for the PVC
	podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: *m.repositoryPVC,
				ReadOnly:  false, // Repository needs write access
			},
		},
	})

	m.logger.V(1).Info("Configured repository PVC",
		"pvc", *m.repositoryPVC,
		"mountPath", repositoryPVCMountPath,
		"repositoryPath", repositoryPath)

	return nil
}

// configureCredentials sets up all credential files in a unified manner to avoid mount conflicts
func (m *Mover) configureCredentials(podSpec *corev1.PodSpec, repo *corev1.Secret) {
	container := &podSpec.Containers[0]
	credentialItems := []corev1.KeyToPath{}
	hasCredentials := false

	// Collect all credential files that exist in the secret
	if _, ok := repo.Data["GOOGLE_APPLICATION_CREDENTIALS"]; ok {
		// GCS credentials - kopia expects this to be an env var pointing to a file
		// We mount the file data from the secret to the expected location
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  "GOOGLE_APPLICATION_CREDENTIALS",
			Value: path.Join(credentialDir, gcsCredentialFile),
		})
		credentialItems = append(credentialItems, corev1.KeyToPath{
			Key:  "GOOGLE_APPLICATION_CREDENTIALS",
			Path: gcsCredentialFile,
		})
		hasCredentials = true
	}

	if _, ok := repo.Data["GOOGLE_DRIVE_CREDENTIALS"]; ok {
		// Google Drive credentials
		driveCredentialFile := "gdrive.json"
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  "GOOGLE_DRIVE_CREDENTIALS",
			Value: path.Join(credentialDir, driveCredentialFile),
		})
		credentialItems = append(credentialItems, corev1.KeyToPath{
			Key:  "GOOGLE_DRIVE_CREDENTIALS",
			Path: driveCredentialFile,
		})
		hasCredentials = true
	}

	if _, ok := repo.Data["SFTP_KEY_FILE"]; ok {
		// SFTP SSH key file
		sshKeyFile := "sftp_key"
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  "SFTP_KEY_FILE",
			Value: path.Join(credentialDir, sshKeyFile),
		})
		credentialItems = append(credentialItems, corev1.KeyToPath{
			Key:  "SFTP_KEY_FILE",
			Path: sshKeyFile,
			Mode: ptr.To[int32](0600), // SSH keys need restrictive permissions
		})
		hasCredentials = true
	}

	// Mount all credential files in a single volume if any credentials exist
	if hasCredentials {
		// Add the volume mount for credentials
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      "credentials",
			MountPath: credentialDir,
			ReadOnly:  true,
		})

		// Create the volume with all credential files
		podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
			Name: "credentials",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  repo.Name,
					Items:       credentialItems,
					DefaultMode: ptr.To[int32](0400), // Default read-only for security
				},
			},
		})
	}
}

// configureSecurityContext sets up security context for privileged/non-privileged mode
func (m *Mover) configureSecurityContext(podSpec *corev1.PodSpec) {
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
}

// handleJobStatus handles job status checking and completion logic
func (m *Mover) handleJobStatus(ctx context.Context, job *batchv1.Job,
	logger logr.Logger, err error) (*batchv1.Job, error) {
	// Record job retries if there are failures
	if job.Status.Failed > 0 && job.Status.Failed < *job.Spec.BackoffLimit {
		operation := operationBackup
		if !m.isSource {
			operation = "restore"
		}
		m.recordJobRetry(operation, "job_pod_failure")
	}

	// If Job had failed, delete it so it can be recreated
	if job.Status.Failed >= *job.Spec.BackoffLimit {
		// Update status with mover logs from failed job using Kopia-specific filter
		logFilter := utils.AllLines
		if !m.isSource {
			// Use Kopia-specific log filter for destinations to extract discovery info
			logFilter = LogFilter
		}
		utils.UpdateMoverStatusForFailedJob(ctx, m.logger, m.latestMoverStatus, job.GetName(), job.GetNamespace(),
			logFilter)

		// For destination jobs, parse logs for discovery information
		if !m.isSource && m.destinationStatus != nil {
			m.updateDestinationDiscoveryStatus()
		}

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
		if m.shouldRunMaintenance() {
			now := metav1.Now()
			m.sourceStatus.LastMaintenance = &now
			logger.Info("maintenance completed", ".Status.Kopia.LastMaintenance", m.sourceStatus.LastMaintenance)
		}
	}

	// update status with mover logs from successful job
	// Use Kopia-specific filter for better log extraction
	logFilter := utils.AllLines
	if !m.isSource {
		logFilter = LogFilter
	}
	utils.UpdateMoverStatusForSuccessfulJob(ctx, m.logger, m.latestMoverStatus, job.GetName(), job.GetNamespace(),
		logFilter)

	// We only continue reconciling if the kopia job has completed
	return job, nil
}

// setupPrerequisites handles the setup of all required resources before running the job
func (m *Mover) setupPrerequisites(ctx context.Context) (*corev1.PersistentVolumeClaim,
	*corev1.PersistentVolumeClaim, *corev1.ServiceAccount, *corev1.Secret,
	utils.CustomCAObject, utils.CustomCAObject, error) {
	// Record cache metrics
	m.recordCacheMetrics()

	// Record policy compliance
	m.recordPolicyCompliance()
	// Allocate temporary data PVC
	var dataPVC *corev1.PersistentVolumeClaim
	var err error
	if m.isSource {
		dataPVC, err = m.ensureSourcePVC(ctx)
	} else {
		dataPVC, err = m.ensureDestinationPVC(ctx)
	}
	if dataPVC == nil || err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}

	// Allocate cache volume
	// cleanupCachePVC will always be false for replicationsources - it's only set in the builder FromDestination()
	// cachePVC can be nil when using EmptyDir fallback
	cachePVC, err := m.ensureCache(ctx, dataPVC, m.cleanupCachePVC)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}

	// Prepare ServiceAccount
	sa, err := m.saHandler.Reconcile(ctx, m.logger)
	if sa == nil || err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}

	// Validate Repository Secret
	repo, err := m.validateRepository(ctx)
	if repo == nil || err != nil {
		m.recordConfigurationError("repository_validation_failed")
		return nil, nil, nil, nil, nil, nil, err
	}

	// Validate custom CA if in spec
	customCAObj, err := utils.ValidateCustomCA(ctx, m.client, m.logger,
		m.owner.GetNamespace(), m.customCASpec)
	// nil customCAObj is ok (indicates we're not using a custom CA)
	if err != nil {
		m.recordConfigurationError("custom_ca_validation_failed")
		return nil, nil, nil, nil, nil, nil, err
	}

	// Validate policy config if in spec
	policyConfigObj, err := m.validatePolicyConfig(ctx)
	// nil policyConfigObj is ok (indicates we're not using custom policies)
	if err != nil {
		m.recordConfigurationError("policy_config_validation_failed")
		return nil, nil, nil, nil, nil, nil, err
	}

	return dataPVC, cachePVC, sa, repo, customCAObj, policyConfigObj, nil
}

// handleJobCompletion processes job status and handles completion logic
func (m *Mover) handleJobCompletion(ctx context.Context, job *batchv1.Job,
	dataPVC *corev1.PersistentVolumeClaim) (mover.Result, error) {
	// Job handling - check for completion or failure
	if job.Status.Failed > 0 {
		// For destination jobs, parse logs for discovery information
		if !m.isSource && m.destinationStatus != nil {
			m.updateDestinationDiscoveryStatus()
		}
		// Job has failed - this should have been handled in ensureJob,
		// but we check here too for safety
		return mover.InProgress(), nil
	}

	// Stop here if the job hasn't completed yet
	if job.Status.Succeeded == 0 {
		return mover.InProgress(), nil
	}

	// Job completed successfully
	// Clear discovery status on successful restore
	if !m.isSource && m.destinationStatus != nil {
		m.destinationStatus.AvailableIdentities = nil
		m.destinationStatus.SnapshotsFound = 0
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

func (m *Mover) shouldRunMaintenance() bool {
	if m.maintenanceInterval == nil || *m.maintenanceInterval <= 0 {
		return false
	}

	if m.sourceStatus == nil || m.sourceStatus.LastMaintenance == nil {
		return true
	}

	lastMaintenance := m.sourceStatus.LastMaintenance.Time
	nextMaintenance := lastMaintenance.Add(time.Duration(*m.maintenanceInterval) * 24 * time.Hour)

	return time.Now().After(nextMaintenance)
}

// recordOperationSuccess records metrics for successful operations
func (m *Mover) recordOperationSuccess(operation string, duration time.Duration) {
	labels := m.getMetricLabels(operation)

	m.metrics.OperationSuccess.With(labels).Inc()
	m.metrics.OperationDuration.With(labels).Observe(duration.Seconds())

	// Record snapshot creation success for backup operations
	if operation == operationBackup {
		m.metrics.SnapshotCreationSuccess.With(labels).Inc()
	}
}

// recordOperationFailure records metrics for failed operations
func (m *Mover) recordOperationFailure(operation, failureReason string) {
	labels := m.getMetricLabels(operation)
	labels["failure_reason"] = failureReason

	m.metrics.OperationFailure.With(labels).Inc()

	// Record snapshot creation failure for backup operations
	if operation == operationBackup {
		m.metrics.SnapshotCreationFailure.With(labels).Inc()
	}
}

// recordMaintenanceOperation records metrics for maintenance operations
func (m *Mover) recordMaintenanceOperation() {
	labels := m.getMetricLabels("maintenance")
	labels["maintenance_type"] = "scheduled"

	m.metrics.MaintenanceOperations.With(labels).Inc()
}

// recordJobRetry records metrics for job retries
func (m *Mover) recordJobRetry(operation, retryReason string) {
	labels := m.getMetricLabels(operation)
	labels["retry_reason"] = retryReason

	m.metrics.JobRetries.With(labels).Inc()
}

// recordCacheMetrics records cache-related metrics
func (m *Mover) recordCacheMetrics() {
	labels := m.getMetricLabels("")

	// Determine cache configuration strategy and record metrics accordingly
	hasPVCConfig := m.cacheStorageClassName != nil || m.cacheAccessModes != nil
	hasCapacityOnly := m.cacheCapacity != nil && !hasPVCConfig
	hasNoCacheConfig := m.cacheCapacity == nil && !hasPVCConfig

	// Record cache type being used
	cacheTypeLabels := make(prometheus.Labels)
	for k, v := range labels {
		cacheTypeLabels[k] = v
	}

	if hasNoCacheConfig || hasCapacityOnly {
		// Using EmptyDir
		cacheTypeLabels["cache_type"] = "emptydir"
		m.metrics.CacheType.With(cacheTypeLabels).Set(1)

		// Reset PVC type to 0
		pvcTypeLabels := make(prometheus.Labels)
		for k, v := range labels {
			pvcTypeLabels[k] = v
		}
		pvcTypeLabels["cache_type"] = "pvc"
		m.metrics.CacheType.With(pvcTypeLabels).Set(0)

		// Record EmptyDir cache size
		var cacheSizeBytes int64
		if m.cacheCapacity != nil {
			// User-specified capacity for EmptyDir
			cacheSizeBytes = m.cacheCapacity.Value()
		} else {
			// Default 8Gi limit for EmptyDir
			defaultLimit := resource.MustParse("8Gi")
			cacheSizeBytes = defaultLimit.Value()
		}
		m.metrics.CacheSize.With(labels).Set(float64(cacheSizeBytes))
	} else {
		// Using PVC
		cacheTypeLabels["cache_type"] = "pvc"
		m.metrics.CacheType.With(cacheTypeLabels).Set(1)

		// Reset EmptyDir type to 0
		emptyDirTypeLabels := make(prometheus.Labels)
		for k, v := range labels {
			emptyDirTypeLabels[k] = v
		}
		emptyDirTypeLabels["cache_type"] = "emptydir"
		m.metrics.CacheType.With(emptyDirTypeLabels).Set(0)

		// Record PVC cache size (if capacity is specified)
		if m.cacheCapacity != nil {
			cacheSizeBytes := m.cacheCapacity.Value()
			m.metrics.CacheSize.With(labels).Set(float64(cacheSizeBytes))
		}
	}
}

// recordPolicyCompliance records policy compliance metrics
func (m *Mover) recordPolicyCompliance() {
	labels := m.getMetricLabels("")

	// Check retention policy compliance (simplified)
	if m.retainPolicy != nil {
		retentionLabels := prometheus.Labels{}
		for k, v := range labels {
			retentionLabels[k] = v
		}

		// Mark retention policies as compliant if configured
		if m.retainPolicy.Hourly != nil {
			retentionLabels["retention_type"] = "hourly"
			m.metrics.RetentionCompliance.With(retentionLabels).Set(1)
		}
		if m.retainPolicy.Daily != nil {
			retentionLabels["retention_type"] = "daily"
			m.metrics.RetentionCompliance.With(retentionLabels).Set(1)
		}
		if m.retainPolicy.Weekly != nil {
			retentionLabels["retention_type"] = "weekly"
			m.metrics.RetentionCompliance.With(retentionLabels).Set(1)
		}
		if m.retainPolicy.Monthly != nil {
			retentionLabels["retention_type"] = "monthly"
			m.metrics.RetentionCompliance.With(retentionLabels).Set(1)
		}
		if m.retainPolicy.Yearly != nil {
			retentionLabels["retention_type"] = "yearly"
			m.metrics.RetentionCompliance.With(retentionLabels).Set(1)
		}
	}

	// Check policy configuration compliance
	policyLabels := prometheus.Labels{}
	for k, v := range labels {
		policyLabels[k] = v
	}

	if m.policyConfig != nil {
		policyLabels["policy_type"] = "global"
		m.metrics.PolicyCompliance.With(policyLabels).Set(1)
	}
}

// recordConfigurationError records configuration error metrics
func (m *Mover) recordConfigurationError(errorType string) {
	labels := m.getMetricLabels("")
	labels["error_type"] = errorType

	m.metrics.ConfigurationErrors.With(labels).Inc()
}

// getMetricLabels returns the base metric labels for this mover instance
func (m *Mover) getMetricLabels(operation string) prometheus.Labels {
	role := "source"
	if !m.isSource {
		role = "destination"
	}

	labels := prometheus.Labels{
		"obj_name":      m.owner.GetName(),
		"obj_namespace": m.owner.GetNamespace(),
		"role":          role,
		"operation":     operation, // Always include operation, even if empty
		"repository":    m.repositoryName,
	}

	return labels
}

// updateDestinationDiscoveryStatus updates the destination status with discovery information
// parsed from failed job logs
func (m *Mover) updateDestinationDiscoveryStatus() {
	if m.destinationStatus == nil || m.latestMoverStatus == nil {
		return
	}

	// Parse the logs for discovery information
	requestedIdentity, availableIdentities, errorMsg := ParseKopiaDiscoveryOutput(m.latestMoverStatus.Logs)

	// Update the destination status with discovery information
	if requestedIdentity != "" {
		m.destinationStatus.RequestedIdentity = requestedIdentity
	}

	if len(availableIdentities) > 0 {
		m.destinationStatus.AvailableIdentities = availableIdentities
		// Count total snapshots for the requested identity
		for _, identity := range availableIdentities {
			if identity.Identity == requestedIdentity {
				m.destinationStatus.SnapshotsFound = identity.SnapshotCount
				break
			}
		}
	}

	// Update the error message with more helpful information
	if errorMsg != "" && m.latestMoverStatus.Result == volsyncv1alpha1.MoverResultFailed {
		// Enhance the error message in the mover status
		if !strings.Contains(m.latestMoverStatus.Logs, errorMsg) {
			m.latestMoverStatus.Logs = errorMsg + "\n\n" + m.latestMoverStatus.Logs
		}
	}

	m.logger.V(1).Info("Updated destination discovery status",
		"requestedIdentity", requestedIdentity,
		"availableIdentities", len(availableIdentities),
		"errorMsg", errorMsg)
}
