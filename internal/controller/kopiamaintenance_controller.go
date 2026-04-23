/*
Copyright 2025 The VolSync authors.

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

package controller

import (
	"context"
	"crypto/sha256"
	"fmt"
	"maps"
	"reflect"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	cron "github.com/robfig/cron/v3"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/internal/controller/utils"
)

const (
	// kopiaMaintenanceFinalizer is the finalizer added to KopiaMaintenance resources
	kopiaMaintenanceFinalizer = "volsync.backube/kopiamaintenance-protection"

	// Maintenance job timeout values
	maintenanceJobTTL           = 3600 // 1 hour
	maintenanceJobTimeout       = 3600 // 1 hour
	maintenanceJobBackoffLimit  = 5
	maintenanceJobCheckInterval = 10 * time.Second

	// Failure thresholds
	maxConsecutiveFailures = 5
	maxScheduledFailures   = 10
)

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// resourceRequirementsEqual compares two ResourceRequirements for equality
// Returns true if both Requests and Limits are equal, false otherwise
func resourceRequirementsEqual(a, b corev1.ResourceRequirements) bool {
	// Compare Requests
	if len(a.Requests) != len(b.Requests) {
		return false
	}
	for resourceName, quantityA := range a.Requests {
		quantityB, exists := b.Requests[resourceName]
		if !exists {
			return false
		}
		// Use Cmp for accurate quantity comparison
		if quantityA.Cmp(quantityB) != 0 {
			return false
		}
	}

	// Compare Limits
	if len(a.Limits) != len(b.Limits) {
		return false
	}
	for resourceName, quantityA := range a.Limits {
		quantityB, exists := b.Limits[resourceName]
		if !exists {
			return false
		}
		// Use Cmp for accurate quantity comparison
		if quantityA.Cmp(quantityB) != 0 {
			return false
		}
	}

	return true
}

// podSecurityContextEqual compares two PodSecurityContext objects for equality
// Returns true if both contexts are semantically equal, false otherwise
func podSecurityContextEqual(a, b *corev1.PodSecurityContext) bool {
	// Handle nil cases
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Compare RunAsUser
	if (a.RunAsUser == nil) != (b.RunAsUser == nil) {
		return false
	}
	if a.RunAsUser != nil && *a.RunAsUser != *b.RunAsUser {
		return false
	}

	// Compare RunAsGroup
	if (a.RunAsGroup == nil) != (b.RunAsGroup == nil) {
		return false
	}
	if a.RunAsGroup != nil && *a.RunAsGroup != *b.RunAsGroup {
		return false
	}

	// Compare FSGroup
	if (a.FSGroup == nil) != (b.FSGroup == nil) {
		return false
	}
	if a.FSGroup != nil && *a.FSGroup != *b.FSGroup {
		return false
	}

	// Compare RunAsNonRoot
	if (a.RunAsNonRoot == nil) != (b.RunAsNonRoot == nil) {
		return false
	}
	if a.RunAsNonRoot != nil && *a.RunAsNonRoot != *b.RunAsNonRoot {
		return false
	}

	// Compare FSGroupChangePolicy
	if (a.FSGroupChangePolicy == nil) != (b.FSGroupChangePolicy == nil) {
		return false
	}
	if a.FSGroupChangePolicy != nil && *a.FSGroupChangePolicy != *b.FSGroupChangePolicy {
		return false
	}

	// Compare SupplementalGroups
	if len(a.SupplementalGroups) != len(b.SupplementalGroups) {
		return false
	}
	for i := range a.SupplementalGroups {
		if a.SupplementalGroups[i] != b.SupplementalGroups[i] {
			return false
		}
	}

	// Compare SELinuxOptions
	if (a.SELinuxOptions == nil) != (b.SELinuxOptions == nil) {
		return false
	}
	if a.SELinuxOptions != nil {
		if a.SELinuxOptions.User != b.SELinuxOptions.User ||
			a.SELinuxOptions.Role != b.SELinuxOptions.Role ||
			a.SELinuxOptions.Type != b.SELinuxOptions.Type ||
			a.SELinuxOptions.Level != b.SELinuxOptions.Level {
			return false
		}
	}

	// Compare Seccomp
	if (a.SeccompProfile == nil) != (b.SeccompProfile == nil) {
		return false
	}
	if a.SeccompProfile != nil {
		if a.SeccompProfile.Type != b.SeccompProfile.Type {
			return false
		}
		if (a.SeccompProfile.LocalhostProfile == nil) != (b.SeccompProfile.LocalhostProfile == nil) {
			return false
		}
		if a.SeccompProfile.LocalhostProfile != nil &&
			*a.SeccompProfile.LocalhostProfile != *b.SeccompProfile.LocalhostProfile {
			return false
		}
	}

	// Compare WindowsOptions
	if (a.WindowsOptions == nil) != (b.WindowsOptions == nil) {
		return false
	}
	if a.WindowsOptions != nil {
		if (a.WindowsOptions.GMSACredentialSpecName == nil) !=
			(b.WindowsOptions.GMSACredentialSpecName == nil) {
			return false
		}
		if a.WindowsOptions.GMSACredentialSpecName != nil &&
			*a.WindowsOptions.GMSACredentialSpecName != *b.WindowsOptions.GMSACredentialSpecName {
			return false
		}
		if (a.WindowsOptions.GMSACredentialSpec == nil) !=
			(b.WindowsOptions.GMSACredentialSpec == nil) {
			return false
		}
		if a.WindowsOptions.GMSACredentialSpec != nil &&
			*a.WindowsOptions.GMSACredentialSpec != *b.WindowsOptions.GMSACredentialSpec {
			return false
		}
		if (a.WindowsOptions.RunAsUserName == nil) != (b.WindowsOptions.RunAsUserName == nil) {
			return false
		}
		if a.WindowsOptions.RunAsUserName != nil && *a.WindowsOptions.RunAsUserName != *b.WindowsOptions.RunAsUserName {
			return false
		}
		if (a.WindowsOptions.HostProcess == nil) != (b.WindowsOptions.HostProcess == nil) {
			return false
		}
		if a.WindowsOptions.HostProcess != nil && *a.WindowsOptions.HostProcess != *b.WindowsOptions.HostProcess {
			return false
		}
	}

	return true
}

// containerSecurityContextEqual compares two SecurityContext objects for equality
// Returns true if both contexts are semantically equal, false otherwise
func containerSecurityContextEqual(a, b *corev1.SecurityContext) bool {
	// Handle nil cases
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Compare RunAsUser
	if (a.RunAsUser == nil) != (b.RunAsUser == nil) {
		return false
	}
	if a.RunAsUser != nil && *a.RunAsUser != *b.RunAsUser {
		return false
	}

	// Compare RunAsGroup
	if (a.RunAsGroup == nil) != (b.RunAsGroup == nil) {
		return false
	}
	if a.RunAsGroup != nil && *a.RunAsGroup != *b.RunAsGroup {
		return false
	}

	// Compare RunAsNonRoot
	if (a.RunAsNonRoot == nil) != (b.RunAsNonRoot == nil) {
		return false
	}
	if a.RunAsNonRoot != nil && *a.RunAsNonRoot != *b.RunAsNonRoot {
		return false
	}

	// Compare ReadOnlyRootFilesystem
	if (a.ReadOnlyRootFilesystem == nil) != (b.ReadOnlyRootFilesystem == nil) {
		return false
	}
	if a.ReadOnlyRootFilesystem != nil && *a.ReadOnlyRootFilesystem != *b.ReadOnlyRootFilesystem {
		return false
	}

	// Compare AllowPrivilegeEscalation
	if (a.AllowPrivilegeEscalation == nil) != (b.AllowPrivilegeEscalation == nil) {
		return false
	}
	if a.AllowPrivilegeEscalation != nil && *a.AllowPrivilegeEscalation != *b.AllowPrivilegeEscalation {
		return false
	}

	// Compare Privileged
	if (a.Privileged == nil) != (b.Privileged == nil) {
		return false
	}
	if a.Privileged != nil && *a.Privileged != *b.Privileged {
		return false
	}

	// Compare Capabilities
	if (a.Capabilities == nil) != (b.Capabilities == nil) {
		return false
	}
	if a.Capabilities != nil {
		if len(a.Capabilities.Add) != len(b.Capabilities.Add) {
			return false
		}
		for i := range a.Capabilities.Add {
			if a.Capabilities.Add[i] != b.Capabilities.Add[i] {
				return false
			}
		}
		if len(a.Capabilities.Drop) != len(b.Capabilities.Drop) {
			return false
		}
		for i := range a.Capabilities.Drop {
			if a.Capabilities.Drop[i] != b.Capabilities.Drop[i] {
				return false
			}
		}
	}

	// Compare SELinuxOptions
	if (a.SELinuxOptions == nil) != (b.SELinuxOptions == nil) {
		return false
	}
	if a.SELinuxOptions != nil {
		if a.SELinuxOptions.User != b.SELinuxOptions.User ||
			a.SELinuxOptions.Role != b.SELinuxOptions.Role ||
			a.SELinuxOptions.Type != b.SELinuxOptions.Type ||
			a.SELinuxOptions.Level != b.SELinuxOptions.Level {
			return false
		}
	}

	// Compare Seccomp
	if (a.SeccompProfile == nil) != (b.SeccompProfile == nil) {
		return false
	}
	if a.SeccompProfile != nil {
		if a.SeccompProfile.Type != b.SeccompProfile.Type {
			return false
		}
		if (a.SeccompProfile.LocalhostProfile == nil) != (b.SeccompProfile.LocalhostProfile == nil) {
			return false
		}
		if a.SeccompProfile.LocalhostProfile != nil &&
			*a.SeccompProfile.LocalhostProfile != *b.SeccompProfile.LocalhostProfile {
			return false
		}
	}

	// Compare WindowsOptions
	if (a.WindowsOptions == nil) != (b.WindowsOptions == nil) {
		return false
	}
	if a.WindowsOptions != nil {
		if (a.WindowsOptions.GMSACredentialSpecName == nil) !=
			(b.WindowsOptions.GMSACredentialSpecName == nil) {
			return false
		}
		if a.WindowsOptions.GMSACredentialSpecName != nil &&
			*a.WindowsOptions.GMSACredentialSpecName != *b.WindowsOptions.GMSACredentialSpecName {
			return false
		}
		if (a.WindowsOptions.GMSACredentialSpec == nil) !=
			(b.WindowsOptions.GMSACredentialSpec == nil) {
			return false
		}
		if a.WindowsOptions.GMSACredentialSpec != nil &&
			*a.WindowsOptions.GMSACredentialSpec != *b.WindowsOptions.GMSACredentialSpec {
			return false
		}
		if (a.WindowsOptions.RunAsUserName == nil) != (b.WindowsOptions.RunAsUserName == nil) {
			return false
		}
		if a.WindowsOptions.RunAsUserName != nil && *a.WindowsOptions.RunAsUserName != *b.WindowsOptions.RunAsUserName {
			return false
		}
		if (a.WindowsOptions.HostProcess == nil) != (b.WindowsOptions.HostProcess == nil) {
			return false
		}
		if a.WindowsOptions.HostProcess != nil && *a.WindowsOptions.HostProcess != *b.WindowsOptions.HostProcess {
			return false
		}
	}

	return true
}

// calculateMaintenanceCacheLimits returns auto-calculated cache limits based on CacheCapacity.
// Returns (0, 0) if no CacheCapacity is set (no auto-calculation).
// Allocates 70% for metadata cache, 20% for content cache, leaving 10% for other files.
func calculateMaintenanceCacheLimits(cacheCapacity *resource.Quantity) (metadataMB, contentMB int32) {
	if cacheCapacity == nil {
		return 0, 0
	}

	capacityBytes := cacheCapacity.Value()
	capacityMB := capacityBytes / (1024 * 1024)

	metadataMB = int32(float64(capacityMB) * 0.70)
	contentMB = int32(float64(capacityMB) * 0.20)
	return metadataMB, contentMB
}

// getCacheLimitEnvVars returns environment variables for Kopia cache size limits.
// Uses explicit values from spec if set, otherwise auto-calculates from CacheCapacity.
func getCacheLimitEnvVars(metadataLimit, contentLimit *int32, cacheCapacity *resource.Quantity) []corev1.EnvVar {
	var envVars []corev1.EnvVar

	metaMB := metadataLimit
	contentMB := contentLimit

	// Auto-calculate if not explicitly set (nil means auto, 0 means unlimited)
	if metaMB == nil || contentMB == nil {
		autoMeta, autoContent := calculateMaintenanceCacheLimits(cacheCapacity)
		if metaMB == nil && autoMeta > 0 {
			metaMB = &autoMeta
		}
		if contentMB == nil && autoContent > 0 {
			contentMB = &autoContent
		}
	}

	if metaMB != nil && *metaMB > 0 {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "KOPIA_METADATA_CACHE_SIZE_LIMIT_MB",
			Value: strconv.Itoa(int(*metaMB)),
		})
	}
	if contentMB != nil && *contentMB > 0 {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "KOPIA_CONTENT_CACHE_SIZE_LIMIT_MB",
			Value: strconv.Itoa(int(*contentMB)),
		})
	}

	return envVars
}

// KopiaMaintenanceReconciler reconciles a KopiaMaintenance object
type KopiaMaintenanceReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	Log            logr.Logger
	EventRecorder  record.EventRecorder
	containerImage string
}

// getContainerImage returns the container image to use for maintenance jobs
func (r *KopiaMaintenanceReconciler) getContainerImage() string {
	if r.containerImage == "" {
		return utils.GetDefaultKopiaImage()
	}
	return r.containerImage
}

// SetupWithManager sets up the controller with the Manager.
func (r *KopiaMaintenanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Initialize container image if not already set
	if r.containerImage == "" {
		r.containerImage = utils.GetDefaultKopiaImage()
	}

	// Watch KopiaMaintenance resources, CronJobs and Jobs they own
	return ctrl.NewControllerManagedBy(mgr).
		Named("kopiamaintenance"). // Explicit name for the controller
		For(&volsyncv1alpha1.KopiaMaintenance{}).
		Owns(&batchv1.CronJob{}).
		Owns(&batchv1.Job{}). // Also watch owned Jobs for manual maintenance
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 3,
		}).
		Complete(r)
}

// +kubebuilder:rbac:groups=volsync.backube,resources=kopiamaintenances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=volsync.backube,resources=kopiamaintenances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=volsync.backube,resources=kopiamaintenances/finalizers,verbs=update
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile is the main reconciliation loop for KopiaMaintenance resources
func (r *KopiaMaintenanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("kopiamaintenance", req.NamespacedName)

	// Fetch the KopiaMaintenance instance
	maintenance := &volsyncv1alpha1.KopiaMaintenance{}
	err := r.Get(ctx, req.NamespacedName, maintenance)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, could have been deleted
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Fix 5: Allow resetting failures via annotation
	if maintenance.Annotations != nil && maintenance.Annotations["volsync.backube/reset-failures"] == "true" {
		if maintenance.Status == nil {
			maintenance.Status = &volsyncv1alpha1.KopiaMaintenanceStatus{}
		}
		maintenance.Status.MaintenanceFailures = 0
		delete(maintenance.Annotations, "volsync.backube/reset-failures")
		if err := r.Update(ctx, maintenance); err != nil {
			return ctrl.Result{}, err
		}
		logger.Info("Reset failure counter via annotation")
		r.EventRecorder.Event(maintenance, corev1.EventTypeNormal, "FailuresReset",
			"Maintenance failure counter has been reset")
	}

	// Validate the KopiaMaintenance configuration
	if err := maintenance.Validate(); err != nil {
		logger.Error(err, "Invalid KopiaMaintenance configuration")
		r.EventRecorder.Event(maintenance, corev1.EventTypeWarning, "ValidationFailed", err.Error())
		if statusErr := r.updateStatusWithError(ctx, maintenance, "", err); statusErr != nil {
			logger.Error(statusErr, "Failed to update status after validation failure")
		}
		return ctrl.Result{}, err
	}

	// Check if the object is being deleted
	if !maintenance.DeletionTimestamp.IsZero() {
		// Handle deletion
		if controllerutil.ContainsFinalizer(maintenance, kopiaMaintenanceFinalizer) {
			logger.Info("Handling deletion")
			// Clean up any existing CronJobs
			if err := r.cleanupCronJob(ctx, maintenance); err != nil {
				logger.Error(err, "Failed to cleanup CronJob during deletion")
				return ctrl.Result{RequeueAfter: 30 * time.Second}, err
			}

			// Remove the finalizer
			controllerutil.RemoveFinalizer(maintenance, kopiaMaintenanceFinalizer)
			if err := r.Update(ctx, maintenance); err != nil {
				logger.Error(err, "Failed to remove finalizer")
				return ctrl.Result{}, err
			}
			logger.Info("Successfully removed finalizer")
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(maintenance, kopiaMaintenanceFinalizer) {
		controllerutil.AddFinalizer(maintenance, kopiaMaintenanceFinalizer)
		if err := r.Update(ctx, maintenance); err != nil {
			logger.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		logger.V(1).Info("Added finalizer")
		return ctrl.Result{Requeue: true}, nil
	}

	// Check if maintenance is enabled
	if !maintenance.GetEnabled() {
		logger.V(1).Info("Maintenance is disabled")
		// Clean up any existing CronJobs/Jobs
		if err := r.cleanupCronJob(ctx, maintenance); err != nil {
			logger.Error(err, "Failed to cleanup CronJob")
			return ctrl.Result{RequeueAfter: time.Minute}, err
		}
		return ctrl.Result{}, r.updateStatusWithError(ctx, maintenance, "", nil)
	}

	// Handle manual trigger
	if maintenance.HasManualTrigger() {
		// Fix 3: Clean up any existing CronJob from scheduled trigger when switching to manual
		if err := r.cleanupCronJob(ctx, maintenance); err != nil {
			logger.Error(err, "Failed to cleanup CronJob when switching to manual trigger")
		}
		// Clean up old manual jobs from previous triggers
		if err := r.cleanupOldManualJobs(ctx, maintenance); err != nil {
			logger.Error(err, "Failed to cleanup old manual jobs")
		}
		return r.handleManualTrigger(ctx, maintenance, logger)
	}

	// Handle scheduled trigger
	if maintenance.HasScheduleTrigger() {
		// Clean up any old manual jobs when switching to scheduled
		if err := r.cleanupOldManualJobs(ctx, maintenance); err != nil {
			logger.Error(err, "Failed to cleanup old manual jobs when switching to scheduled trigger")
		}
		// Check for excessive failures in scheduled maintenance
		if maintenance.Status != nil && maintenance.Status.MaintenanceFailures >= maxScheduledFailures {
			logger.Info("Scheduled maintenance has failed too many times",
				"failures", maintenance.Status.MaintenanceFailures,
				"maxFailures", maxScheduledFailures)
			r.EventRecorder.Event(maintenance, corev1.EventTypeWarning, "ExcessiveScheduledFailures",
				fmt.Sprintf("Scheduled maintenance has failed %d times. Consider checking repository configuration and credentials.",
					maintenance.Status.MaintenanceFailures))
		}

		// Ensure the CronJob exists for the repository
		cronJobName, err := r.ensureCronJob(ctx, maintenance)
		if err != nil {
			logger.Error(err, "Failed to ensure CronJob")
			r.EventRecorder.Event(maintenance, corev1.EventTypeWarning, "CronJobFailed",
				fmt.Sprintf("Failed to ensure CronJob: %v", err))
			// Update status with error
			if statusErr := r.updateStatusWithError(ctx, maintenance, "", err); statusErr != nil {
				logger.Error(statusErr, "Failed to update status")
			}
			return ctrl.Result{RequeueAfter: time.Minute}, err
		}

		// Update status
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, r.updateStatusWithError(ctx, maintenance, cronJobName, nil)
	}

	// No trigger configured, use default schedule
	logger.V(1).Info("No trigger configured, using default schedule")
	cronJobName, err := r.ensureCronJob(ctx, maintenance)
	if err != nil {
		logger.Error(err, "Failed to ensure CronJob")
		r.EventRecorder.Event(maintenance, corev1.EventTypeWarning, "CronJobFailed",
			fmt.Sprintf("Failed to ensure CronJob: %v", err))
		// Update status with error
		if statusErr := r.updateStatusWithError(ctx, maintenance, "", err); statusErr != nil {
			logger.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: time.Minute}, err
	}

	// Update status
	return ctrl.Result{RequeueAfter: 5 * time.Minute}, r.updateStatusWithError(ctx, maintenance, cronJobName, nil)
}

// handleManualTrigger handles manual trigger for KopiaMaintenance
func (r *KopiaMaintenanceReconciler) handleManualTrigger(ctx context.Context, maintenance *volsyncv1alpha1.KopiaMaintenance, logger logr.Logger) (ctrl.Result, error) {
	manualTag := maintenance.GetManualTrigger()

	// Check if we need to trigger a maintenance job
	if maintenance.Status != nil && maintenance.Status.LastManualSync == manualTag {
		// Already synced with this manual tag
		logger.V(1).Info("Manual trigger already processed", "tag", manualTag)
		// Don't requeue - the trigger has been processed. User must change or remove the trigger.
		return ctrl.Result{}, nil
	}

	logger.Info("Processing manual trigger", "tag", manualTag)

	// Fix 2: Check if job already exists for this trigger before creating a new one
	hashInput := fmt.Sprintf("%s/%s/%s", maintenance.Namespace, maintenance.Name, manualTag)
	hash := sha256.Sum256([]byte(hashInput))
	expectedJobName := fmt.Sprintf("kopia-maint-manual-%x", hash[:8])

	existingJob := &batchv1.Job{}
	jobExists := false
	if err := r.Get(ctx, types.NamespacedName{Name: expectedJobName, Namespace: maintenance.Namespace}, existingJob); err == nil {
		jobExists = true
		logger.V(1).Info("Job already exists for this manual trigger", "job", expectedJobName)
	}

	// Fix 4: Actually stop after 5 failures (don't create job, just block)
	if maintenance.Status != nil && maintenance.Status.MaintenanceFailures >= maxConsecutiveFailures {
		logger.Error(nil, "Too many consecutive maintenance failures, blocking job creation",
			"failures", maintenance.Status.MaintenanceFailures,
			"maxFailures", maxConsecutiveFailures)

		// Mark as processed but DON'T create job
		maintenance.Status.LastManualSync = manualTag
		r.EventRecorder.Event(maintenance, corev1.EventTypeWarning, "MaintenanceBlocked",
			fmt.Sprintf("Blocked due to %d failures. Add annotation 'volsync.backube/reset-failures=true' to reset.",
				maintenance.Status.MaintenanceFailures))

		// Update status and return WITHOUT creating job
		if err := r.updateStatusWithError(ctx, maintenance, "",
			fmt.Errorf("blocked after %d failures", maintenance.Status.MaintenanceFailures)); err != nil {
			logger.Error(err, "Failed to update status")
		}

		// Don't requeue, don't create job - wait for user to reset failures
		return ctrl.Result{}, nil
	}

	// Create a Job for manual maintenance only if it doesn't exist
	jobName := expectedJobName
	var job *batchv1.Job

	if !jobExists {
		var err error
		jobName, err = r.ensureMaintenanceJob(ctx, maintenance)
		if err != nil {
			logger.Error(err, "Failed to create maintenance job for manual trigger")
			r.EventRecorder.Event(maintenance, corev1.EventTypeWarning, "JobCreationFailed",
				fmt.Sprintf("Failed to create maintenance job: %v", err))
			if statusErr := r.updateStatusWithError(ctx, maintenance, "", err); statusErr != nil {
				logger.Error(statusErr, "Failed to update status")
			}
			return ctrl.Result{RequeueAfter: time.Minute}, err
		}

		// Get the newly created job
		job = &batchv1.Job{}
		if err := r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: maintenance.Namespace}, job); err != nil {
			logger.Error(err, "Failed to get maintenance job")
			return ctrl.Result{RequeueAfter: 10 * time.Second}, err
		}
	} else {
		// Use the existing job
		job = existingJob
	}

	// Check if job completed successfully
	if job.Status.Succeeded > 0 {
		logger.Info("Manual maintenance job completed successfully", "job", jobName)

		// Update LastManualSync in status
		if maintenance.Status == nil {
			maintenance.Status = &volsyncv1alpha1.KopiaMaintenanceStatus{}
		}
		maintenance.Status.LastManualSync = manualTag
		maintenance.Status.LastMaintenanceTime = &metav1.Time{Time: time.Now()}
		maintenance.Status.MaintenanceFailures = 0

		if err := r.updateStatusWithError(ctx, maintenance, "", nil); err != nil {
			logger.Error(err, "Failed to update status after successful manual maintenance")
			return ctrl.Result{RequeueAfter: maintenanceJobCheckInterval}, err
		}

		r.EventRecorder.Event(maintenance, corev1.EventTypeNormal, "ManualMaintenanceCompleted",
			fmt.Sprintf("Manual maintenance completed successfully with tag: %s", manualTag))

		// Don't delete the job immediately - let TTL or history limits handle cleanup
		// This prevents recreating the job if the manual trigger remains in the spec

		// Don't requeue - the manual trigger has been processed
		return ctrl.Result{}, nil
	}

	// Check if job failed
	if job.Status.Failed > 0 {
		logger.Error(nil, "Manual maintenance job failed", "job", jobName)

		if maintenance.Status == nil {
			maintenance.Status = &volsyncv1alpha1.KopiaMaintenanceStatus{}
		}
		maintenance.Status.MaintenanceFailures++

		if err := r.updateStatusWithError(ctx, maintenance, "", fmt.Errorf("maintenance job failed")); err != nil {
			logger.Error(err, "Failed to update status after failed manual maintenance")
		}

		r.EventRecorder.Event(maintenance, corev1.EventTypeWarning, "ManualMaintenanceFailed",
			fmt.Sprintf("Manual maintenance failed with tag: %s", manualTag))

		// Clean up the failed job
		if err := r.Delete(ctx, job); err != nil && !apierrors.IsNotFound(err) {
			logger.Error(err, "Failed to delete failed job", "job", jobName)
		}

		// Mark manual trigger as processed even on failure to avoid infinite retries
		maintenance.Status.LastManualSync = manualTag
		if err := r.updateStatusWithError(ctx, maintenance, "", nil); err != nil {
			logger.Error(err, "Failed to update LastManualSync after failure")
		}

		return ctrl.Result{RequeueAfter: time.Minute}, fmt.Errorf("maintenance job failed")
	}

	// Job is still running
	logger.V(1).Info("Manual maintenance job still running", "job", jobName)
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// ensureMaintenanceJob creates a Job for manual maintenance
func (r *KopiaMaintenanceReconciler) ensureMaintenanceJob(ctx context.Context, maintenance *volsyncv1alpha1.KopiaMaintenance) (string, error) {
	// Verify that the repository secret exists
	repositorySecret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      maintenance.GetRepositorySecret(),
		Namespace: maintenance.Namespace,
	}, repositorySecret); err != nil {
		if apierrors.IsNotFound(err) {
			return "", fmt.Errorf("repository secret %s not found in namespace %s",
				maintenance.GetRepositorySecret(), maintenance.Namespace)
		}
		return "", fmt.Errorf("failed to get repository secret: %w", err)
	}

	// Fix 1: Remove timestamp from job name to prevent infinite loop
	// Generate a deterministic name based on maintenance name and trigger value
	hashInput := fmt.Sprintf("%s/%s/%s", maintenance.Namespace, maintenance.Name,
		maintenance.GetManualTrigger())
	hash := sha256.Sum256([]byte(hashInput))
	jobName := fmt.Sprintf("kopia-maint-manual-%x", hash[:8])

	// Check if job already exists
	existingJob := &batchv1.Job{}
	err := r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: maintenance.Namespace}, existingJob)
	if err == nil {
		// Job already exists
		return jobName, nil
	}
	if !apierrors.IsNotFound(err) {
		return "", fmt.Errorf("failed to check for existing job: %w", err)
	}

	// Create the Job spec similar to what would be in a CronJob
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: maintenance.Namespace,
			Labels: map[string]string{
				"volsync.backube/kopia-maintenance": "true",
				"volsync.backube/maintenance-type":  "manual",
				"volsync.backube/maintenance-name":  maintenance.Name,
			},
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: maintenance.Spec.MoverPodLabels,
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					SecurityContext: func() *corev1.PodSecurityContext {
						if maintenance.Spec.PodSecurityContext != nil {
							return maintenance.Spec.PodSecurityContext
						}
						// Default security context
						return &corev1.PodSecurityContext{
							RunAsNonRoot: ptr.To(true),
							FSGroup:      ptr.To(int64(1000)),
							RunAsUser:    ptr.To(int64(1000)),
						}
					}(),
					Containers: []corev1.Container{
						{
							Name:            "kopia-maintenance",
							Image:           r.getContainerImage(),
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command:         []string{"/mover-kopia/entry.sh"},
							Env: append([]corev1.EnvVar{
								{
									Name:  "DIRECTION",
									Value: "maintenance",
								},
								{
									Name:  "DATA_DIR",
									Value: "/data", // Not used for maintenance but entry.sh expects it
								},
								{
									Name:  "KOPIA_CACHE_DIR",
									Value: "/cache",
								},
							}, getCacheLimitEnvVars(
								maintenance.Spec.MetadataCacheSizeLimitMB,
								maintenance.Spec.ContentCacheSizeLimitMB,
								maintenance.Spec.CacheCapacity,
							)...),
							EnvFrom: []corev1.EnvFromSource{
								{
									SecretRef: &corev1.SecretEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: maintenance.GetRepositorySecret(),
										},
									},
								},
							},
							Resources: func() corev1.ResourceRequirements {
								if maintenance.Spec.Resources != nil {
									return *maintenance.Spec.Resources
								}
								// No default resources - users can set them via spec.resources if needed
								return corev1.ResourceRequirements{}
							}(),
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: ptr.To(false),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
								Privileged:             ptr.To(false),
								ReadOnlyRootFilesystem: ptr.To(true),
								RunAsNonRoot:           ptr.To(true),
								RunAsUser:              ptr.To(int64(1000)),
							},
						},
					},
					Affinity: maintenance.Spec.Affinity,
				},
			},
			// Limit retry attempts to prevent infinite pod creation
			// After 5 failed attempts, the job will be marked as failed
			BackoffLimit: ptr.To(int32(5)),
			// Exponential backoff configuration
			// Starts at 10 seconds and doubles up to 6 times (max ~10 minutes)
			ActiveDeadlineSeconds:   ptr.To(maintenance.GetActiveDeadlineSeconds()), // Configurable timeout (default: 3 hours)
			TTLSecondsAfterFinished: ptr.To(int32(3600)),                            // Clean up after 1 hour
		},
	}

	// Add cache support
	cachePVC, err := r.ensureCache(ctx, maintenance)
	if err != nil {
		return "", fmt.Errorf("failed to ensure cache: %w", err)
	}
	r.configureCacheVolume(&job.Spec.Template.Spec, cachePVC, maintenance)

	// Set owner reference so the job is cleaned up when KopiaMaintenance is deleted
	if err := controllerutil.SetControllerReference(maintenance, job, r.Scheme); err != nil {
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Create the job
	if err := r.Create(ctx, job); err != nil {
		return "", fmt.Errorf("failed to create job: %w", err)
	}

	r.Log.Info("Created manual maintenance job", "job", jobName)
	return jobName, nil
}

// ensureCronJob creates or updates a CronJob for the KopiaMaintenance
func (r *KopiaMaintenanceReconciler) ensureCronJob(ctx context.Context, maintenance *volsyncv1alpha1.KopiaMaintenance) (string, error) {
	// Verify that the repository secret exists
	repositorySecret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      maintenance.GetRepositorySecret(),
		Namespace: maintenance.Namespace,
	}, repositorySecret); err != nil {
		if apierrors.IsNotFound(err) {
			return "", fmt.Errorf("repository secret %s not found in namespace %s",
				maintenance.GetRepositorySecret(), maintenance.Namespace)
		}
		return "", fmt.Errorf("failed to get repository secret: %w", err)
	}

	// Generate a unique name for the CronJob
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s/%s", maintenance.Namespace, maintenance.Name)))
	maxNameLength := 34
	truncatedName := maintenance.Name
	if len(truncatedName) > maxNameLength {
		truncatedName = truncatedName[:maxNameLength]
	}
	cronJobName := fmt.Sprintf("kopia-maint-%s-%x", truncatedName, hash[:8])

	// Check if CronJob already exists
	existingCronJob := &batchv1.CronJob{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      cronJobName,
		Namespace: maintenance.Namespace,
	}, existingCronJob)

	if err == nil {
		// CronJob exists, update it if needed
		updateNeeded := false

		// Check if schedule needs updating
		if existingCronJob.Spec.Schedule != maintenance.GetSchedule() {
			existingCronJob.Spec.Schedule = maintenance.GetSchedule()
			updateNeeded = true
		}

		// Fix 6: Suspend CronJob after too many failures
		if maintenance.Status != nil && maintenance.Status.MaintenanceFailures >= maxConsecutiveFailures {
			if existingCronJob.Spec.Suspend == nil || !*existingCronJob.Spec.Suspend {
				suspend := true
				existingCronJob.Spec.Suspend = &suspend
				updateNeeded = true
				r.EventRecorder.Event(maintenance, corev1.EventTypeWarning, "CronJobSuspended",
					fmt.Sprintf("CronJob suspended due to %d consecutive failures. Add annotation 'volsync.backube/reset-failures=true' to reset.",
						maintenance.Status.MaintenanceFailures))
			}
		} else {
			// Check if suspend state needs updating based on spec
			suspend := maintenance.Spec.Suspend != nil && *maintenance.Spec.Suspend
			if existingCronJob.Spec.Suspend != nil && *existingCronJob.Spec.Suspend != suspend {
				existingCronJob.Spec.Suspend = &suspend
				updateNeeded = true
			} else if existingCronJob.Spec.Suspend == nil && suspend {
				existingCronJob.Spec.Suspend = &suspend
				updateNeeded = true
			}
		}

		// Update history limits if changed
		if maintenance.Spec.SuccessfulJobsHistoryLimit != nil &&
			(existingCronJob.Spec.SuccessfulJobsHistoryLimit == nil ||
				*existingCronJob.Spec.SuccessfulJobsHistoryLimit != *maintenance.Spec.SuccessfulJobsHistoryLimit) {
			existingCronJob.Spec.SuccessfulJobsHistoryLimit = maintenance.Spec.SuccessfulJobsHistoryLimit
			updateNeeded = true
		}

		if maintenance.Spec.FailedJobsHistoryLimit != nil &&
			(existingCronJob.Spec.FailedJobsHistoryLimit == nil ||
				*existingCronJob.Spec.FailedJobsHistoryLimit != *maintenance.Spec.FailedJobsHistoryLimit) {
			existingCronJob.Spec.FailedJobsHistoryLimit = maintenance.Spec.FailedJobsHistoryLimit
			updateNeeded = true
		}

		// Check if resources need updating
		desiredResources := corev1.ResourceRequirements{}
		if maintenance.Spec.Resources != nil {
			desiredResources = *maintenance.Spec.Resources
		}

		// Compare with existing container resources
		if len(existingCronJob.Spec.JobTemplate.Spec.Template.Spec.Containers) > 0 {
			existingResources := existingCronJob.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Resources
			if !resourceRequirementsEqual(existingResources, desiredResources) {
				existingCronJob.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Resources = desiredResources
				updateNeeded = true
				r.Log.Info("Updating maintenance CronJob resources",
					"name", cronJobName,
					"oldResources", existingResources,
					"newResources", desiredResources)
			}
		}

		// Check if PodSecurityContext needs updating
		desiredPodSecurityContext := maintenance.Spec.PodSecurityContext
		if desiredPodSecurityContext == nil {
			// Default security context
			desiredPodSecurityContext = &corev1.PodSecurityContext{
				RunAsNonRoot: ptr.To(true),
				FSGroup:      ptr.To(int64(1000)),
				RunAsUser:    ptr.To(int64(1000)),
			}
		}

		existingPodSecurityContext := existingCronJob.Spec.JobTemplate.Spec.Template.Spec.SecurityContext
		if !podSecurityContextEqual(existingPodSecurityContext, desiredPodSecurityContext) {
			existingCronJob.Spec.JobTemplate.Spec.Template.Spec.SecurityContext = desiredPodSecurityContext
			updateNeeded = true
			r.Log.Info("Updating maintenance CronJob PodSecurityContext",
				"name", cronJobName,
				"oldContext", existingPodSecurityContext,
				"newContext", desiredPodSecurityContext)
		}

		// Check if ContainerSecurityContext needs updating
		desiredContainerSecurityContext := maintenance.Spec.ContainerSecurityContext
		if desiredContainerSecurityContext == nil {
			// Default security context - runAsUser inherited from pod SecurityContext
			desiredContainerSecurityContext = &corev1.SecurityContext{
				AllowPrivilegeEscalation: ptr.To(false),
				Capabilities: &corev1.Capabilities{
					Drop: []corev1.Capability{"ALL"},
				},
				Privileged:             ptr.To(false),
				ReadOnlyRootFilesystem: ptr.To(true),
				RunAsNonRoot:           ptr.To(true),
			}
		}

		if len(existingCronJob.Spec.JobTemplate.Spec.Template.Spec.Containers) > 0 {
			existingContainerSecurityContext := existingCronJob.Spec.JobTemplate.Spec.Template.Spec.Containers[0].SecurityContext
			if !containerSecurityContextEqual(existingContainerSecurityContext, desiredContainerSecurityContext) {
				existingCronJob.Spec.JobTemplate.Spec.Template.Spec.Containers[0].SecurityContext = desiredContainerSecurityContext
				updateNeeded = true
				r.Log.Info("Updating maintenance CronJob ContainerSecurityContext",
					"name", cronJobName,
					"oldContext", existingContainerSecurityContext,
					"newContext", desiredContainerSecurityContext)
			}
		}

		// Check if Container Image needs updating
		desiredImage := r.getContainerImage()
		if len(existingCronJob.Spec.JobTemplate.Spec.Template.Spec.Containers) > 0 {
			existingImage := existingCronJob.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Image
			if existingImage != desiredImage {
				existingCronJob.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Image = desiredImage
				updateNeeded = true
				r.Log.Info("Updating maintenance CronJob Container Image",
					"name", cronJobName,
					"oldImage", existingImage,
					"newImage", desiredImage)
			}
		}

		// Check if ActiveDeadlineSeconds needs updating
		desiredActiveDeadlineSeconds := maintenance.GetActiveDeadlineSeconds()
		existingActiveDeadlineSeconds := existingCronJob.Spec.JobTemplate.Spec.ActiveDeadlineSeconds
		if existingActiveDeadlineSeconds == nil || *existingActiveDeadlineSeconds != desiredActiveDeadlineSeconds {
			existingCronJob.Spec.JobTemplate.Spec.ActiveDeadlineSeconds = ptr.To(desiredActiveDeadlineSeconds)
			updateNeeded = true
			r.Log.Info("Updating maintenance CronJob ActiveDeadlineSeconds",
				"name", cronJobName,
				"oldActiveDeadlineSeconds", existingActiveDeadlineSeconds,
				"newActiveDeadlineSeconds", desiredActiveDeadlineSeconds)
		}

		// Check if ServiceAccountName needs updating
		desiredServiceAccountName := "default"
		if maintenance.Spec.ServiceAccountName != nil {
			desiredServiceAccountName = *maintenance.Spec.ServiceAccountName
		}
		existingServiceAccountName := existingCronJob.Spec.JobTemplate.Spec.Template.Spec.ServiceAccountName
		if existingServiceAccountName != desiredServiceAccountName {
			existingCronJob.Spec.JobTemplate.Spec.Template.Spec.ServiceAccountName = desiredServiceAccountName
			updateNeeded = true
			r.Log.Info("Updating maintenance CronJob ServiceAccountName",
				"name", cronJobName,
				"oldServiceAccountName", existingServiceAccountName,
				"newServiceAccountName", desiredServiceAccountName)
		}

		// Check if MoverPodLabels need updating
		desiredLabels := map[string]string{
			"volsync.backube/kopia-maintenance": "true",
			"volsync.backube/maintenance-name":  maintenance.Name,
		}
		for k, v := range maintenance.Spec.MoverPodLabels {
			desiredLabels[k] = v
		}
		existingLabels := existingCronJob.Spec.JobTemplate.Spec.Template.ObjectMeta.Labels
		if !maps.Equal(existingLabels, desiredLabels) {
			existingCronJob.Spec.JobTemplate.Spec.Template.ObjectMeta.Labels = desiredLabels
			updateNeeded = true
			r.Log.Info("Updating maintenance CronJob MoverPodLabels",
				"name", cronJobName,
				"oldLabels", existingLabels,
				"newLabels", desiredLabels)
		}

		// Check if Affinity needs updating
		desiredAffinity := maintenance.Spec.Affinity
		existingAffinity := existingCronJob.Spec.JobTemplate.Spec.Template.Spec.Affinity
		if !reflect.DeepEqual(existingAffinity, desiredAffinity) {
			existingCronJob.Spec.JobTemplate.Spec.Template.Spec.Affinity = desiredAffinity
			updateNeeded = true
			r.Log.Info("Updating maintenance CronJob Affinity",
				"name", cronJobName,
				"oldAffinity", existingAffinity,
				"newAffinity", desiredAffinity)
		}

		// Check if NodeSelector needs updating
		desiredNodeSelector := maintenance.Spec.NodeSelector
		existingNodeSelector := existingCronJob.Spec.JobTemplate.Spec.Template.Spec.NodeSelector
		if !maps.Equal(existingNodeSelector, desiredNodeSelector) {
			existingCronJob.Spec.JobTemplate.Spec.Template.Spec.NodeSelector = desiredNodeSelector
			updateNeeded = true
			r.Log.Info("Updating maintenance CronJob NodeSelector",
				"name", cronJobName,
				"oldNodeSelector", existingNodeSelector,
				"newNodeSelector", desiredNodeSelector)
		}

		// Check if Tolerations need updating
		desiredTolerations := maintenance.Spec.Tolerations
		existingTolerations := existingCronJob.Spec.JobTemplate.Spec.Template.Spec.Tolerations
		if !reflect.DeepEqual(existingTolerations, desiredTolerations) {
			existingCronJob.Spec.JobTemplate.Spec.Template.Spec.Tolerations = desiredTolerations
			updateNeeded = true
			r.Log.Info("Updating maintenance CronJob Tolerations",
				"name", cronJobName,
				"oldTolerations", existingTolerations,
				"newTolerations", desiredTolerations)
		}

		if updateNeeded {
			if err := r.Update(ctx, existingCronJob); err != nil {
				return "", fmt.Errorf("failed to update CronJob: %w", err)
			}
			r.Log.Info("Updated maintenance CronJob", "name", cronJobName)
		}

		return cronJobName, nil
	}

	if !apierrors.IsNotFound(err) {
		return "", fmt.Errorf("failed to get CronJob: %w", err)
	}

	// Create new CronJob
	cronJob, err := r.buildMaintenanceCronJob(ctx, maintenance, cronJobName)
	if err != nil {
		return "", fmt.Errorf("failed to build CronJob: %w", err)
	}

	// Set owner reference so the CronJob is cleaned up when KopiaMaintenance is deleted
	if err := controllerutil.SetControllerReference(maintenance, cronJob, r.Scheme); err != nil {
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	if err := r.Create(ctx, cronJob); err != nil {
		return "", fmt.Errorf("failed to create CronJob: %w", err)
	}

	r.Log.Info("Created maintenance CronJob", "name", cronJobName, "schedule", maintenance.GetSchedule())
	r.EventRecorder.Event(maintenance, corev1.EventTypeNormal, "CronJobCreated",
		fmt.Sprintf("Created maintenance CronJob %s", cronJobName))

	return cronJobName, nil
}

// buildMaintenanceCronJob creates a CronJob spec for Kopia maintenance
func (r *KopiaMaintenanceReconciler) buildMaintenanceCronJob(ctx context.Context, maintenance *volsyncv1alpha1.KopiaMaintenance, cronJobName string) (*batchv1.CronJob, error) {
	// Determine resources - no defaults, users can set via spec.resources if needed
	resources := corev1.ResourceRequirements{}
	if maintenance.Spec.Resources != nil {
		resources = *maintenance.Spec.Resources
	}

	// Build environment variables
	envVars := []corev1.EnvVar{
		{
			Name:  "DIRECTION",
			Value: "maintenance",
		},
		{
			Name:  "DATA_DIR",
			Value: "/data", // Not used for maintenance but entry.sh expects it
		},
		{
			Name:  "KOPIA_CACHE_DIR",
			Value: "/cache",
		},
	}

	// Add cache limit env vars
	cacheLimitEnvVars := getCacheLimitEnvVars(
		maintenance.Spec.MetadataCacheSizeLimitMB,
		maintenance.Spec.ContentCacheSizeLimitMB,
		maintenance.Spec.CacheCapacity,
	)
	envVars = append(envVars, cacheLimitEnvVars...)

	// Build volumes and volume mounts
	volumes := []corev1.Volume{
		{
			Name: "tmp",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "tmp",
			MountPath: "/tmp",
		},
	}

	// Add CustomCA volumes if specified
	if maintenance.Spec.Repository.CustomCA != nil {
		if maintenance.Spec.Repository.CustomCA.ConfigMapName != "" {
			volumes = append(volumes, corev1.Volume{
				Name: "custom-ca-configmap",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: maintenance.Spec.Repository.CustomCA.ConfigMapName,
						},
						Items: []corev1.KeyToPath{
							{
								Key:  maintenance.Spec.Repository.CustomCA.Key,
								Path: "ca-bundle.crt",
							},
						},
					},
				},
			})
			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				Name:      "custom-ca-configmap",
				MountPath: "/etc/ssl/custom-ca-from-configmap",
				ReadOnly:  true,
			})
			envVars = append(envVars, corev1.EnvVar{
				Name:  "KOPIA_CUSTOM_CA",
				Value: "/etc/ssl/custom-ca-from-configmap/ca-bundle.crt",
			})
		}

		if maintenance.Spec.Repository.CustomCA.SecretName != "" {
			volumes = append(volumes, corev1.Volume{
				Name: "custom-ca-secret",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: maintenance.Spec.Repository.CustomCA.SecretName,
						Items: []corev1.KeyToPath{
							{
								Key:  maintenance.Spec.Repository.CustomCA.Key,
								Path: "ca-bundle.crt",
							},
						},
					},
				},
			})
			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				Name:      "custom-ca-secret",
				MountPath: "/etc/ssl/custom-ca-from-secret",
				ReadOnly:  true,
			})
			envVars = append(envVars, corev1.EnvVar{
				Name:  "KOPIA_CUSTOM_CA",
				Value: "/etc/ssl/custom-ca-from-secret/ca-bundle.crt",
			})
		}
	}

	// Determine suspend state - Fix 6: also suspend on excessive failures
	suspend := (maintenance.Spec.Suspend != nil && *maintenance.Spec.Suspend) ||
		(maintenance.Status != nil && maintenance.Status.MaintenanceFailures >= maxConsecutiveFailures)

	// Build the CronJob
	cronJob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cronJobName,
			Namespace: maintenance.Namespace,
			Labels: map[string]string{
				"volsync.backube/kopia-maintenance": "true",
				"volsync.backube/maintenance-name":  maintenance.Name,
				"app.kubernetes.io/name":            "volsync",
				"app.kubernetes.io/component":       "kopia-maintenance",
				"app.kubernetes.io/managed-by":      "volsync",
			},
		},
		Spec: batchv1.CronJobSpec{
			Schedule:                   maintenance.GetSchedule(),
			ConcurrencyPolicy:          batchv1.ForbidConcurrent,
			Suspend:                    &suspend,
			SuccessfulJobsHistoryLimit: ptr.To(int32(3)),
			FailedJobsHistoryLimit:     ptr.To(int32(1)),
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					// Limit retry attempts to prevent infinite pod creation
					// After 5 failed attempts, the job will be marked as failed
					BackoffLimit: ptr.To(int32(5)),
					// Overall timeout for the job
					ActiveDeadlineSeconds: ptr.To(maintenance.GetActiveDeadlineSeconds()), // Configurable timeout (default: 3 hours)
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: func() map[string]string {
								labels := map[string]string{
									"volsync.backube/kopia-maintenance": "true",
									"volsync.backube/maintenance-name":  maintenance.Name,
								}
								// Add user-specified labels
								for k, v := range maintenance.Spec.MoverPodLabels {
									labels[k] = v
								}
								return labels
							}(),
						},
						Spec: corev1.PodSpec{
							ServiceAccountName: func() string {
								if maintenance.Spec.ServiceAccountName != nil {
									return *maintenance.Spec.ServiceAccountName
								}
								return "default"
							}(),
							RestartPolicy: corev1.RestartPolicyOnFailure,
							SecurityContext: func() *corev1.PodSecurityContext {
								if maintenance.Spec.PodSecurityContext != nil {
									return maintenance.Spec.PodSecurityContext
								}
								// Default security context
								return &corev1.PodSecurityContext{
									RunAsNonRoot: ptr.To(true),
									FSGroup:      ptr.To(int64(1000)),
									RunAsUser:    ptr.To(int64(1000)),
								}
							}(),
							Containers: []corev1.Container{
								{
									Name:            "kopia-maintenance",
									Image:           r.getContainerImage(),
									ImagePullPolicy: corev1.PullIfNotPresent,
									Command:         []string{"/mover-kopia/entry.sh"},
									Env:             envVars,
									EnvFrom: []corev1.EnvFromSource{
										{
											SecretRef: &corev1.SecretEnvSource{
												LocalObjectReference: corev1.LocalObjectReference{
													Name: maintenance.GetRepositorySecret(),
												},
											},
										},
									},
									VolumeMounts: volumeMounts,
									Resources:    resources,
									SecurityContext: func() *corev1.SecurityContext {
										if maintenance.Spec.ContainerSecurityContext != nil {
											return maintenance.Spec.ContainerSecurityContext
										}
										// Default security context - runAsUser inherited from pod SecurityContext
										return &corev1.SecurityContext{
											AllowPrivilegeEscalation: ptr.To(false),
											Capabilities: &corev1.Capabilities{
												Drop: []corev1.Capability{"ALL"},
											},
											Privileged:             ptr.To(false),
											ReadOnlyRootFilesystem: ptr.To(true),
											RunAsNonRoot:           ptr.To(true),
										}
									}(),
								},
							},
							Volumes:      volumes,
							Affinity:     maintenance.Spec.Affinity,
							NodeSelector: maintenance.Spec.NodeSelector,
							Tolerations:  maintenance.Spec.Tolerations,
						},
					},
				},
			},
		},
	}

	// Add cache support
	cachePVC, err := r.ensureCache(ctx, maintenance)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure cache: %w", err)
	}
	r.configureCacheVolume(&cronJob.Spec.JobTemplate.Spec.Template.Spec, cachePVC, maintenance)

	// Apply history limits if specified
	if maintenance.Spec.SuccessfulJobsHistoryLimit != nil {
		cronJob.Spec.SuccessfulJobsHistoryLimit = maintenance.Spec.SuccessfulJobsHistoryLimit
	}
	if maintenance.Spec.FailedJobsHistoryLimit != nil {
		cronJob.Spec.FailedJobsHistoryLimit = maintenance.Spec.FailedJobsHistoryLimit
	}

	return cronJob, nil
}

// cleanupCronJob removes the CronJob managed by this KopiaMaintenance
func (r *KopiaMaintenanceReconciler) cleanupCronJob(ctx context.Context, maintenance *volsyncv1alpha1.KopiaMaintenance) error {
	// The CronJob should be automatically deleted due to owner references
	// But we can try to delete it explicitly if needed
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s/%s", maintenance.Namespace, maintenance.Name)))

	// Use same truncation logic as in ensureCronJob
	maxNameLength := 34
	truncatedName := maintenance.Name
	if len(truncatedName) > maxNameLength {
		truncatedName = truncatedName[:maxNameLength]
	}
	cronJobName := fmt.Sprintf("kopia-maint-%s-%x", truncatedName, hash[:8])
	cronJob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cronJobName,
			Namespace: maintenance.Namespace,
		},
	}

	if err := r.Delete(ctx, cronJob); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete CronJob: %w", err)
	}

	return nil
}

// updateStatusWithError updates the KopiaMaintenance status including error conditions
func (r *KopiaMaintenanceReconciler) updateStatusWithError(ctx context.Context, maintenance *volsyncv1alpha1.KopiaMaintenance, activeCronJob string, reconcileErr error) error {
	// Get the latest version of the resource from the cluster for accurate patch base
	original := &volsyncv1alpha1.KopiaMaintenance{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      maintenance.Name,
		Namespace: maintenance.Namespace,
	}, original); err != nil {
		// If we can't get the original, fall back to using the passed-in object
		original = maintenance.DeepCopy()
	}

	// Ensure status is initialized
	if maintenance.Status == nil {
		maintenance.Status = &volsyncv1alpha1.KopiaMaintenanceStatus{
			Conditions: []metav1.Condition{},
		}
	}

	// Update ObservedGeneration
	maintenance.Status.ObservedGeneration = maintenance.Generation

	// Update active CronJob
	maintenance.Status.ActiveCronJob = activeCronJob

	// Update last reconcile time
	maintenance.Status.LastReconcileTime = &metav1.Time{Time: time.Now()}

	// Check if there's an active CronJob and get its status
	if activeCronJob != "" {
		cronJob := &batchv1.CronJob{}
		if err := r.Get(ctx, types.NamespacedName{
			Name:      activeCronJob,
			Namespace: maintenance.Namespace,
		}, cronJob); err == nil {
			// Calculate the actual next scheduled maintenance time using cron parser
			if schedule := maintenance.GetSchedule(); schedule != "" {
				if nextTime, err := r.calculateNextScheduledTime(schedule); err == nil {
					maintenance.Status.NextScheduledMaintenance = &metav1.Time{Time: nextTime}
				} else {
					r.Log.V(1).Info("Failed to parse cron schedule", "schedule", schedule, "error", err)
					// Fallback to approximate calculation
					if cronJob.Status.LastScheduleTime != nil {
						nextTime := cronJob.Status.LastScheduleTime.Add(24 * time.Hour)
						maintenance.Status.NextScheduledMaintenance = &metav1.Time{Time: nextTime}
					}
				}
			}

			// Check for recent job completions
			if cronJob.Status.LastSuccessfulTime != nil {
				maintenance.Status.LastMaintenanceTime = cronJob.Status.LastSuccessfulTime
				// Reset failure counter on success
				maintenance.Status.MaintenanceFailures = 0
			}

			// Check for failed jobs and track consecutive failures
			jobs := &batchv1.JobList{}
			if err := r.List(ctx, jobs,
				client.InNamespace(maintenance.Namespace),
				client.MatchingLabels{
					"volsync.backube/kopia-maintenance": "true",
					"volsync.backube/maintenance-name":  maintenance.Name,
				}); err == nil {
				// Count recent failed jobs (within last 24 hours)
				recentFailures := int32(0)
				cutoffTime := time.Now().Add(-24 * time.Hour)
				for _, job := range jobs.Items {
					if job.CreationTimestamp.After(cutoffTime) {
						// Check if job has failed (exceeded backoff limit)
						if job.Status.Failed > 0 && job.Status.Conditions != nil {
							for _, condition := range job.Status.Conditions {
								if condition.Type == batchv1.JobFailed &&
									condition.Status == corev1.ConditionTrue &&
									(condition.Reason == "BackoffLimitExceeded" || condition.Reason == "DeadlineExceeded") {
									recentFailures++
									break
								}
							}
						}
					}
				}

				// Update failure count if we have recent failures and no recent success
				if recentFailures > 0 && (cronJob.Status.LastSuccessfulTime == nil ||
					cronJob.Status.LastSuccessfulTime.Before(&metav1.Time{Time: cutoffTime})) {
					maintenance.Status.MaintenanceFailures = recentFailures
					r.Log.V(1).Info("Detected recent maintenance failures", "failures", recentFailures)
				}
			}
		}
	}

	// Update conditions
	r.updateConditions(maintenance, activeCronJob, reconcileErr)

	// Use Patch instead of Update for status
	patch := client.MergeFrom(original)
	if err := r.Status().Patch(ctx, maintenance, patch); err != nil {
		r.Log.Error(err, "Failed to patch KopiaMaintenance status",
			"namespace", maintenance.Namespace, "name", maintenance.Name)
		return fmt.Errorf("failed to patch status: %w", err)
	}

	return nil
}

// updateConditions updates the status conditions
func (r *KopiaMaintenanceReconciler) updateConditions(maintenance *volsyncv1alpha1.KopiaMaintenance, activeCronJob string, reconcileErr error) {
	// Progressing condition - follows Kubernetes deployment/statefulset pattern
	progressingCondition := metav1.Condition{
		Type:               "Progressing",
		ObservedGeneration: maintenance.Generation,
	}

	if reconcileErr != nil {
		// If there's an error, we're still progressing (retrying)
		progressingCondition.Status = metav1.ConditionTrue
		progressingCondition.Reason = "ReconcileError"
		progressingCondition.Message = fmt.Sprintf("Reconciliation in progress, error: %v", reconcileErr)
	} else if maintenance.Status.ObservedGeneration < maintenance.Generation {
		// New generation observed, processing update
		progressingCondition.Status = metav1.ConditionTrue
		progressingCondition.Reason = "NewGenerationObserved"
		progressingCondition.Message = "Processing spec update"
	} else if !maintenance.GetEnabled() {
		// Disabled state is stable, not progressing
		progressingCondition.Status = metav1.ConditionFalse
		progressingCondition.Reason = "MaintenanceDisabled"
		progressingCondition.Message = "Maintenance is disabled"
	} else if activeCronJob != "" {
		// Successfully created/updated, stable state
		progressingCondition.Status = metav1.ConditionFalse
		progressingCondition.Reason = "ReconcileComplete"
		progressingCondition.Message = "CronJob successfully configured"
	} else {
		// Still trying to create CronJob
		progressingCondition.Status = metav1.ConditionTrue
		progressingCondition.Reason = "CreatingCronJob"
		progressingCondition.Message = "Creating maintenance CronJob"
	}

	// Ready condition
	readyCondition := metav1.Condition{
		Type:               "Ready",
		ObservedGeneration: maintenance.Generation,
	}

	if reconcileErr != nil {
		readyCondition.Status = metav1.ConditionFalse
		readyCondition.Reason = "ReconcileFailed"
		readyCondition.Message = fmt.Sprintf("Reconcile failed: %v", reconcileErr)
	} else if !maintenance.GetEnabled() {
		readyCondition.Status = metav1.ConditionFalse
		readyCondition.Reason = "MaintenanceDisabled"
		readyCondition.Message = "Maintenance is disabled"
	} else if activeCronJob != "" {
		readyCondition.Status = metav1.ConditionTrue
		readyCondition.Reason = "MaintenanceActive"
		readyCondition.Message = fmt.Sprintf("Maintenance CronJob %s is active for repository %s",
			activeCronJob, maintenance.GetRepositorySecret())
	} else {
		readyCondition.Status = metav1.ConditionFalse
		readyCondition.Reason = "CronJobCreationFailed"
		readyCondition.Message = "Failed to create maintenance CronJob"
	}

	// Add a condition for excessive failures
	if maintenance.Status != nil && maintenance.Status.MaintenanceFailures >= 5 {
		failureCondition := metav1.Condition{
			Type:               "MaintenanceHealthy",
			ObservedGeneration: maintenance.Generation,
			Status:             metav1.ConditionFalse,
			Reason:             "ExcessiveFailures",
			Message:            fmt.Sprintf("Maintenance has failed %d times. Manual intervention may be required.", maintenance.Status.MaintenanceFailures),
		}
		apimeta.SetStatusCondition(&maintenance.Status.Conditions, failureCondition)
	} else {
		healthyCondition := metav1.Condition{
			Type:               "MaintenanceHealthy",
			ObservedGeneration: maintenance.Generation,
			Status:             metav1.ConditionTrue,
			Reason:             "OperatingNormally",
			Message:            "Maintenance is operating normally",
		}
		apimeta.SetStatusCondition(&maintenance.Status.Conditions, healthyCondition)
	}

	// Use apimeta.SetStatusCondition for proper condition management
	apimeta.SetStatusCondition(&maintenance.Status.Conditions, progressingCondition)
	apimeta.SetStatusCondition(&maintenance.Status.Conditions, readyCondition)
}

// calculateNextScheduledTime calculates the next scheduled time based on the cron expression
func (r *KopiaMaintenanceReconciler) calculateNextScheduledTime(schedule string) (time.Time, error) {
	// Parse the cron schedule
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	sched, err := parser.Parse(schedule)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse cron schedule: %w", err)
	}

	// Get the next scheduled time
	now := time.Now()
	next := sched.Next(now)
	return next, nil
}

// ensureCache ensures the cache PVC exists if configured, or returns nil for EmptyDir fallback
func (r *KopiaMaintenanceReconciler) ensureCache(ctx context.Context, km *volsyncv1alpha1.KopiaMaintenance) (*corev1.PersistentVolumeClaim, error) {
	// Check if an existing PVC is specified
	if km.Spec.CachePVC != nil && *km.Spec.CachePVC != "" {
		pvc := &corev1.PersistentVolumeClaim{}
		if err := r.Get(ctx, types.NamespacedName{
			Name:      *km.Spec.CachePVC,
			Namespace: km.Namespace,
		}, pvc); err != nil {
			return nil, fmt.Errorf("failed to get cache PVC %s: %w", *km.Spec.CachePVC, err)
		}
		return pvc, nil
	}

	// Determine cache configuration scenario
	hasPVCConfig := km.Spec.CacheStorageClassName != nil || km.Spec.CacheAccessModes != nil
	hasCapacityOnly := km.Spec.CacheCapacity != nil && !hasPVCConfig
	hasNoCacheConfig := km.Spec.CacheCapacity == nil && !hasPVCConfig

	// Use EmptyDir fallback for scenarios 1 and 2
	if hasNoCacheConfig || hasCapacityOnly {
		return nil, nil // nil PVC means EmptyDir will be used
	}

	// Scenario 3: Create PVC
	cacheName := fmt.Sprintf("volsync-kopia-maintenance-%s-cache", km.Name)

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cacheName,
			Namespace: km.Namespace,
		},
	}

	// Check if PVC already exists
	err := r.Get(ctx, client.ObjectKeyFromObject(pvc), pvc)
	if err == nil {
		return pvc, nil // PVC exists
	}
	if !kerrors.IsNotFound(err) {
		return nil, err
	}

	// Create new PVC
	capacity := resource.MustParse("1Gi")
	if km.Spec.CacheCapacity != nil {
		capacity = *km.Spec.CacheCapacity
	}

	accessModes := km.Spec.CacheAccessModes
	if len(accessModes) == 0 {
		accessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
	}

	pvc.Spec = corev1.PersistentVolumeClaimSpec{
		AccessModes: accessModes,
		Resources: corev1.VolumeResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: capacity,
			},
		},
		StorageClassName: km.Spec.CacheStorageClassName,
	}

	if err := ctrl.SetControllerReference(km, pvc, r.Scheme); err != nil {
		return nil, err
	}

	if err := r.Create(ctx, pvc); err != nil {
		return nil, err
	}

	return pvc, nil
}

// cleanupOldManualJobs removes old manual jobs when trigger changes - Fix 7
func (r *KopiaMaintenanceReconciler) cleanupOldManualJobs(ctx context.Context, maintenance *volsyncv1alpha1.KopiaMaintenance) error {
	// List all manual jobs for this maintenance
	jobs := &batchv1.JobList{}
	if err := r.List(ctx, jobs,
		client.InNamespace(maintenance.Namespace),
		client.MatchingLabels{
			"volsync.backube/kopia-maintenance": "true",
			"volsync.backube/maintenance-type":  "manual",
			"volsync.backube/maintenance-name":  maintenance.Name,
		}); err != nil {
		return err
	}

	// Delete old jobs except the current one (if it matches the current trigger)
	currentJobName := ""
	if maintenance.HasManualTrigger() {
		// Calculate what the current job name would be
		hashInput := fmt.Sprintf("%s/%s/%s", maintenance.Namespace, maintenance.Name,
			maintenance.GetManualTrigger())
		hash := sha256.Sum256([]byte(hashInput))
		currentJobName = fmt.Sprintf("kopia-maint-manual-%x", hash[:8])
	}

	for _, job := range jobs.Items {
		// Don't delete the current job if it exists
		if job.Name == currentJobName {
			continue
		}
		// Delete old job
		if err := r.Delete(ctx, &job); err != nil && !apierrors.IsNotFound(err) {
			r.Log.V(1).Info("Failed to delete old manual job", "job", job.Name, "error", err)
		}
	}
	return nil
}

// configureCacheVolume configures the cache volume on the pod spec
func (r *KopiaMaintenanceReconciler) configureCacheVolume(podSpec *corev1.PodSpec, cachePVC *corev1.PersistentVolumeClaim, km *volsyncv1alpha1.KopiaMaintenance) {
	// Add volume mount to container
	podSpec.Containers[0].VolumeMounts = append(podSpec.Containers[0].VolumeMounts, corev1.VolumeMount{
		Name:      "kopia-cache",
		MountPath: "/cache",
	})

	var cacheVolume corev1.Volume
	if cachePVC != nil {
		// Use PVC when cache is configured
		cacheVolume = corev1.Volume{
			Name: "kopia-cache",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: cachePVC.Name,
				},
			},
		}
	} else {
		// Use EmptyDir as fallback
		emptyDirSource := &corev1.EmptyDirVolumeSource{}

		// Set size limit based on configuration
		if km.Spec.CacheCapacity != nil {
			emptyDirSource.SizeLimit = km.Spec.CacheCapacity
		} else {
			defaultLimit := resource.MustParse("8Gi")
			emptyDirSource.SizeLimit = &defaultLimit
		}

		cacheVolume = corev1.Volume{
			Name: "kopia-cache",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: emptyDirSource,
			},
		}
	}

	podSpec.Volumes = append(podSpec.Volumes, cacheVolume)
}
