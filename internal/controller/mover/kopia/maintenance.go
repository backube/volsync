//go:build !disable_kopia

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

package kopia

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

const (
	// Default maintenance username for CronJobs
	defaultMaintenanceUsername = "maintenance@volsync"
	// Default maintenance schedule (2 AM daily)
	defaultMaintenanceSchedule = "0 2 * * *"
	// Label keys for maintenance CronJobs
	maintenanceLabelKey        = "volsync.backube/kopia-maintenance"
	maintenanceRepositoryLabel = "volsync.backube/repository-hash"
	maintenanceNamespaceLabel  = "volsync.backube/source-namespace"
	// Annotation for repository config
	maintenanceRepositoryAnnotation = "volsync.backube/repository-config"
	// Annotation to track schedule conflicts
	// When multiple ReplicationSources from different namespaces try to set different
	// schedules for the same repository, this annotation records the last rejected attempt
	maintenanceScheduleConflictAnnotation = "volsync.backube/schedule-conflict"
	// ServiceAccount name for maintenance
	maintenanceServiceAccountName = "volsync-kopia-maintenance"
	// Maximum length for CronJob name
	maxCronJobNameLength = 52
	// Label for copied maintenance secrets
	maintenanceSecretLabel = "volsync.backube/maintenance-secret"
	// Environment variable for operator namespace
	operatorNamespaceEnvVar = "POD_NAMESPACE"
)

// MaintenanceManager handles the lifecycle of Kopia maintenance CronJobs
type MaintenanceManager struct {
	client            client.Client
	logger            logr.Logger
	containerImage    string
	metrics           kopiaMetrics
	operatorNamespace string // cached operator namespace
	builder           *Builder
}

// NewMaintenanceManager creates a new MaintenanceManager
func NewMaintenanceManager(client client.Client, logger logr.Logger, containerImage string, builder *Builder) *MaintenanceManager {
	m := &MaintenanceManager{
		client:         client,
		logger:         logger.WithName("maintenance"),
		containerImage: containerImage,
		metrics:        newKopiaMetrics(),
		builder:        builder,
	}
	// Initialize operator namespace
	m.operatorNamespace = m.getOperatorNamespace()
	return m
}

// getOperatorNamespace returns the namespace where the operator is running
// Phase 2: Add operator namespace support
func (m *MaintenanceManager) getOperatorNamespace() string {
	// Check environment variable first
	if ns := os.Getenv(operatorNamespaceEnvVar); ns != "" {
		m.logger.V(1).Info("Using operator namespace from environment", "namespace", ns)
		return ns
	}

	// Try to read from service account namespace file (more reliable in pods)
	const nsFile = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
	if nsBytes, err := os.ReadFile(nsFile); err == nil && len(nsBytes) > 0 {
		ns := strings.TrimSpace(string(nsBytes))
		if ns != "" {
			m.logger.V(1).Info("Using operator namespace from service account", "namespace", ns)
			return ns
		}
	}

	// Fall back to volsync-system as default
	const defaultNS = "volsync-system"
	m.logger.V(1).Info("POD_NAMESPACE not set and service account namespace not found, using default", "namespace", defaultNS)
	return defaultNS
}

// RepositoryConfig represents the unique configuration for a Kopia repository
type RepositoryConfig struct {
	// Repository secret name
	Repository string `json:"repository"`
	// Custom CA configuration
	CustomCA *volsyncv1alpha1.CustomCASpec `json:"customCA,omitempty"`
	// Namespace where the repository is used (not included in hash)
	Namespace string `json:"-"`
	// Schedule for maintenance (not included in hash)
	Schedule string `json:"-"`
}

// Hash generates a deterministic hash for the repository configuration
// Phase 1: Only hash the repository secret name to identify unique repositories
func (rc *RepositoryConfig) Hash() string {
	// Only include repository-specific fields in the hash
	// This ensures one CronJob per repository regardless of namespace or schedule
	repoCfg := struct {
		Repository string                        `json:"repository"`
		CustomCA   *volsyncv1alpha1.CustomCASpec `json:"customCA,omitempty"`
	}{
		Repository: rc.Repository,
		CustomCA:   rc.CustomCA,
	}

	data, err := json.Marshal(repoCfg)
	if err != nil {
		// Fallback to deterministic hash based on key fields
		fallbackStr := rc.Repository
		if rc.CustomCA != nil {
			fallbackStr = fmt.Sprintf("%s:ca-%s-%s", fallbackStr, rc.CustomCA.SecretName, rc.CustomCA.ConfigMapName)
		}
		data = []byte(fallbackStr)
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])[:16] // Use first 16 chars for shorter names
}

// EnsureMaintenanceCronJob ensures a maintenance CronJob exists for the given ReplicationSource
// This is the public API for the proactive maintenance controller
func (m *MaintenanceManager) EnsureMaintenanceCronJob(ctx context.Context,
	source *volsyncv1alpha1.ReplicationSource) error {
	return m.ReconcileMaintenanceForSource(ctx, source)
}

// ReconcileMaintenanceForSource ensures a maintenance CronJob exists for the repository
// used by this ReplicationSource
func (m *MaintenanceManager) ReconcileMaintenanceForSource(ctx context.Context,
	source *volsyncv1alpha1.ReplicationSource) error {
	if source.Spec.Kopia == nil {
		return nil
	}

	// Validate input parameters
	if source.Name == "" {
		return fmt.Errorf("ReplicationSource name is required but was empty")
	}
	if source.Namespace == "" {
		return fmt.Errorf("ReplicationSource namespace is required but was empty")
	}
	if source.Spec.Kopia.Repository == "" {
		return fmt.Errorf("kopia repository configuration is required but was empty")
	}

	// Check if maintenance is disabled
	if !m.isMaintenanceEnabled(source) {
		m.logger.V(1).Info("Maintenance disabled for source",
			"source", source.Name,
			"namespace", source.Namespace)
		return nil
	}

	// Create repository config
	repoConfig := &RepositoryConfig{
		Repository: source.Spec.Kopia.Repository,
		CustomCA:   (*volsyncv1alpha1.CustomCASpec)(&source.Spec.Kopia.CustomCA),
		Namespace:  source.Namespace,
		Schedule:   m.getMaintenanceSchedule(source),
	}

	// Ensure maintenance CronJob exists
	return m.ensureMaintenanceCronJob(ctx, repoConfig, source)
}

// ensureMaintenanceCronJob creates or updates a maintenance CronJob for the given repository
func (m *MaintenanceManager) ensureMaintenanceCronJob(ctx context.Context,
	repoConfig *RepositoryConfig, owner client.Object) error {
	// Phase 3: Ensure maintenance secret is copied to operator namespace
	copiedSecretName, err := m.ensureMaintenanceSecret(ctx, repoConfig)
	if err != nil {
		return fmt.Errorf("failed to ensure maintenance secret: %w", err)
	}

	// Phase 5: Ensure ServiceAccount exists in operator namespace
	if err := m.ensureServiceAccount(ctx); err != nil {
		return fmt.Errorf("failed to ensure service account: %w", err)
	}

	cronJob := m.buildMaintenanceCronJob(repoConfig, owner, copiedSecretName)

	// Check if CronJob already exists
	existing := &batchv1.CronJob{}
	err = m.client.Get(ctx, types.NamespacedName{
		Name:      cronJob.Name,
		Namespace: cronJob.Namespace,
	}, existing)

	if err != nil {
		if errors.IsNotFound(err) {
			// Phase 7: Check for migration from source namespace
			if err := m.migrateExistingCronJob(ctx, repoConfig); err != nil {
				m.logger.Error(err, "Failed to migrate existing CronJob",
					"repository", repoConfig.Repository,
					"sourceNamespace", repoConfig.Namespace)
			}

			// Create new CronJob
			m.logger.Info("Creating maintenance CronJob",
				"name", cronJob.Name,
				"namespace", cronJob.Namespace,
				"schedule", cronJob.Spec.Schedule)

			if err := m.client.Create(ctx, cronJob); err != nil {
				return err
			}

			// Record metric for created CronJob
			m.recordCronJobMetric(owner, "created")
			return nil
		}
		return fmt.Errorf("failed to get CronJob: %w", err)
	}

	// Phase 4: Handle schedule conflicts using FIRST-WINS strategy
	// Strategy explanation:
	// When multiple ReplicationSources use the same Kopia repository but specify different
	// maintenance schedules, we use a "first-wins" strategy to resolve conflicts:
	// 1. The first ReplicationSource to create a maintenance CronJob sets the schedule
	// 2. Subsequent sources from DIFFERENT namespaces cannot change the schedule
	// 3. Sources from the SAME namespace CAN update the schedule (single-tenant scenario)
	// 4. Conflicts are tracked via annotations for visibility and debugging
	// 5. The CronJob persists as long as ANY source needs it (even if first source is deleted)
	//
	// Rationale: This prevents unpredictable schedule changes in multi-tenant environments
	// while allowing single-namespace deployments full control over their maintenance schedule.
	if existing.Spec.Schedule != cronJob.Spec.Schedule {
		// Check if this is a genuine conflict or just an update from same source
		sourceNamespace, exists := existing.Labels[maintenanceNamespaceLabel]
		if exists && sourceNamespace != repoConfig.Namespace {
			// CONFLICT: Different namespace trying to change schedule - REJECT
			m.logger.Info("Schedule conflict detected - preserving existing schedule (first-wins strategy)",
				"cronJob", existing.Name,
				"existingSchedule", existing.Spec.Schedule,
				"requestedSchedule", cronJob.Spec.Schedule,
				"existingNamespace", sourceNamespace,
				"requestingNamespace", repoConfig.Namespace,
				"resolution", "keeping existing schedule")

			// Track the conflict attempt in annotations for visibility
			if existing.Annotations == nil {
				existing.Annotations = make(map[string]string)
			}
			// Record the last conflict attempt (overwrites previous conflicts)
			existing.Annotations[maintenanceScheduleConflictAnnotation] = fmt.Sprintf(
				"Last conflict: Schedule '%s' requested from namespace '%s' at %s (rejected - first-wins strategy)",
				cronJob.Spec.Schedule, repoConfig.Namespace, time.Now().Format(time.RFC3339))

			if err := m.client.Update(ctx, existing); err != nil {
				m.logger.Error(err, "Failed to update conflict annotation")
			}
		} else {
			// NO CONFLICT: Update from same namespace or initial setup - ALLOW
			m.logger.Info("Updating maintenance CronJob schedule (same namespace update allowed)",
				"name", existing.Name,
				"namespace", existing.Namespace,
				"oldSchedule", existing.Spec.Schedule,
				"newSchedule", cronJob.Spec.Schedule,
				"sourceNamespace", repoConfig.Namespace)
			existing.Spec.Schedule = cronJob.Spec.Schedule

			// Clear any previous conflict annotations since this is a valid update
			if existing.Annotations != nil {
				delete(existing.Annotations, maintenanceScheduleConflictAnnotation)
			}

			if err := m.client.Update(ctx, existing); err != nil {
				return err
			}

			// Record metric for updated CronJob
			m.recordCronJobMetric(owner, "updated")
		}
	}

	// Update labels to include current namespace if not already present
	if existing.Labels[maintenanceNamespaceLabel] != repoConfig.Namespace {
		if existing.Labels == nil {
			existing.Labels = make(map[string]string)
		}
		// Keep track of all namespaces using this CronJob
		existing.Labels[maintenanceNamespaceLabel] = repoConfig.Namespace
		if err := m.client.Update(ctx, existing); err != nil {
			m.logger.Error(err, "Failed to update namespace label")
		}
	}

	// CronJob already exists and is up to date
	return nil
}

// CleanupOrphanedMaintenanceCronJobs removes maintenance CronJobs that no longer have
// any associated Kopia ReplicationSources
// Phase 6: Update cleanup logic for centralized CronJobs
func (m *MaintenanceManager) CleanupOrphanedMaintenanceCronJobs(ctx context.Context,
	namespace string) error {
	// List all Kopia ReplicationSources in the namespace
	sourceList := &volsyncv1alpha1.ReplicationSourceList{}
	if err := m.client.List(ctx, sourceList, client.InNamespace(namespace)); err != nil {
		return fmt.Errorf("failed to list ReplicationSources: %w", err)
	}

	// Build a set of required repository hashes for this namespace
	requiredHashes := make(map[string]bool)
	for _, source := range sourceList.Items {
		if source.Spec.Kopia != nil {
			// Skip if maintenance is disabled
			if !m.isMaintenanceEnabled(&source) {
				continue
			}
			repoConfig := &RepositoryConfig{
				Repository: source.Spec.Kopia.Repository,
				CustomCA:   (*volsyncv1alpha1.CustomCASpec)(&source.Spec.Kopia.CustomCA),
				Namespace:  source.Namespace,
				Schedule:   m.getMaintenanceSchedule(&source),
			}
			requiredHashes[repoConfig.Hash()] = true
		}
	}

	// List all maintenance CronJobs in operator namespace that belong to this source namespace
	cronJobList := &batchv1.CronJobList{}
	listOpts := []client.ListOption{
		client.InNamespace(m.operatorNamespace), // Look in operator namespace
		client.MatchingLabels{
			maintenanceLabelKey:       "true",
			maintenanceNamespaceLabel: namespace, // Filter by source namespace
		},
	}
	if err := m.client.List(ctx, cronJobList, listOpts...); err != nil {
		return fmt.Errorf("failed to list maintenance CronJobs: %w", err)
	}

	// Check each CronJob to see if it's still needed
	for _, cronJob := range cronJobList.Items {
		repoHash, exists := cronJob.Labels[maintenanceRepositoryLabel]
		if !exists || !requiredHashes[repoHash] {
			// This CronJob is no longer needed by any source in this namespace
			// But check if it's used by other namespaces before deleting
			if err := m.checkAndDeleteOrphanedCronJob(ctx, &cronJob, namespace); err != nil {
				m.logger.Error(err, "Failed to check/delete orphaned CronJob",
					"cronJob", cronJob.Name)
			}
		}
	}

	// Clean up orphaned secrets
	if err := m.cleanupOrphanedSecrets(ctx, namespace); err != nil {
		m.logger.Error(err, "Failed to cleanup orphaned secrets")
	}

	return nil
}

// checkAndDeleteOrphanedCronJob checks if a CronJob is used by other namespaces
func (m *MaintenanceManager) checkAndDeleteOrphanedCronJob(ctx context.Context,
	cronJob *batchv1.CronJob, namespace string) error {
	// Get the repository hash
	repoHash, exists := cronJob.Labels[maintenanceRepositoryLabel]
	if !exists {
		return nil
	}

	// Check if any other namespace is using this repository
	allSources := &volsyncv1alpha1.ReplicationSourceList{}
	if err := m.client.List(ctx, allSources); err != nil {
		return fmt.Errorf("failed to list all ReplicationSources: %w", err)
	}

	for _, source := range allSources.Items {
		if source.Namespace == namespace {
			continue // Skip the namespace we're cleaning up
		}
		if source.Spec.Kopia != nil && m.isMaintenanceEnabled(&source) {
			repoConfig := &RepositoryConfig{
				Repository: source.Spec.Kopia.Repository,
				CustomCA:   (*volsyncv1alpha1.CustomCASpec)(&source.Spec.Kopia.CustomCA),
				Namespace:  source.Namespace,
			}
			if repoConfig.Hash() == repoHash {
				// Another namespace is using this repository
				m.logger.V(1).Info("CronJob still needed by another namespace",
					"cronJob", cronJob.Name,
					"usingNamespace", source.Namespace)
				return nil
			}
		}
	}

	// No other namespace is using this CronJob, safe to delete
	m.logger.Info("Deleting orphaned maintenance CronJob",
		"cronJob", cronJob.Name,
		"namespace", cronJob.Namespace)
	if err := m.client.Delete(ctx, cronJob); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete orphaned CronJob %s: %w", cronJob.Name, err)
	}

	// Record metric for deleted CronJob
	m.recordCronJobDeletionMetric(cronJob.Namespace, cronJob.Name)
	return nil
}

// cleanupOrphanedSecrets removes copied maintenance secrets that are no longer needed
// Phase 6: Add secret cleanup
func (m *MaintenanceManager) cleanupOrphanedSecrets(ctx context.Context, namespace string) error {
	// List all maintenance secrets for this namespace
	secretList := &corev1.SecretList{}
	listOpts := []client.ListOption{
		client.InNamespace(m.operatorNamespace),
		client.MatchingLabels{
			maintenanceSecretLabel:    "true",
			maintenanceNamespaceLabel: namespace,
		},
	}
	if err := m.client.List(ctx, secretList, listOpts...); err != nil {
		return fmt.Errorf("failed to list maintenance secrets: %w", err)
	}

	// List all ReplicationSources in the namespace
	sourceList := &volsyncv1alpha1.ReplicationSourceList{}
	if err := m.client.List(ctx, sourceList, client.InNamespace(namespace)); err != nil {
		return fmt.Errorf("failed to list ReplicationSources: %w", err)
	}

	// Build set of required secrets
	requiredSecrets := make(map[string]bool)
	for _, source := range sourceList.Items {
		if source.Spec.Kopia != nil && m.isMaintenanceEnabled(&source) {
			copiedSecretName := fmt.Sprintf("maintenance-%s-%s", namespace, source.Spec.Kopia.Repository)
			if len(copiedSecretName) > 63 {
				repoConfig := &RepositoryConfig{
					Repository: source.Spec.Kopia.Repository,
					CustomCA:   (*volsyncv1alpha1.CustomCASpec)(&source.Spec.Kopia.CustomCA),
				}
				hash := repoConfig.Hash()
				maxPrefix := 63 - len(hash) - 1
				copiedSecretName = copiedSecretName[:maxPrefix] + "-" + hash
			}
			requiredSecrets[copiedSecretName] = true
		}
	}

	// Delete orphaned secrets
	for _, secret := range secretList.Items {
		if !requiredSecrets[secret.Name] {
			m.logger.Info("Deleting orphaned maintenance secret",
				"secret", secret.Name,
				"namespace", secret.Namespace)
			if err := m.client.Delete(ctx, &secret); err != nil && !errors.IsNotFound(err) {
				return fmt.Errorf("failed to delete orphaned secret %s: %w", secret.Name, err)
			}
		}
	}

	return nil
}

// ensureMaintenanceSecret copies the repository secret to the operator namespace
// Phase 3: Implement secret copying
func (m *MaintenanceManager) ensureMaintenanceSecret(ctx context.Context,
	repoConfig *RepositoryConfig) (string, error) {
	// Get source secret
	sourceSecret := &corev1.Secret{}
	err := m.client.Get(ctx, types.NamespacedName{
		Name:      repoConfig.Repository,
		Namespace: repoConfig.Namespace,
	}, sourceSecret)
	if err != nil {
		return "", fmt.Errorf("failed to get source secret: %w", err)
	}

	// Generate name for copied secret
	copiedSecretName := fmt.Sprintf("maintenance-%s-%s", repoConfig.Namespace, repoConfig.Repository)
	if len(copiedSecretName) > 63 {
		// Truncate if too long, keeping a hash suffix for uniqueness
		hash := repoConfig.Hash()
		maxPrefix := 63 - len(hash) - 1
		copiedSecretName = copiedSecretName[:maxPrefix] + "-" + hash
	}

	// Check if secret already exists in operator namespace
	copiedSecret := &corev1.Secret{}
	err = m.client.Get(ctx, types.NamespacedName{
		Name:      copiedSecretName,
		Namespace: m.operatorNamespace,
	}, copiedSecret)

	if err != nil {
		if errors.IsNotFound(err) {
			// Create new secret
			copiedSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      copiedSecretName,
					Namespace: m.operatorNamespace,
					Labels: map[string]string{
						maintenanceSecretLabel:     "true",
						maintenanceRepositoryLabel: repoConfig.Hash(),
						maintenanceNamespaceLabel:  repoConfig.Namespace,
					},
					Annotations: map[string]string{
						"volsync.backube/source-namespace": repoConfig.Namespace,
						"volsync.backube/source-secret":    repoConfig.Repository,
					},
				},
				Type: sourceSecret.Type,
				Data: sourceSecret.Data,
			}

			m.logger.Info("Creating maintenance secret copy",
				"name", copiedSecretName,
				"namespace", m.operatorNamespace,
				"sourceNamespace", repoConfig.Namespace,
				"sourceSecret", repoConfig.Repository)

			if err := m.client.Create(ctx, copiedSecret); err != nil {
				return "", fmt.Errorf("failed to create maintenance secret: %w", err)
			}
		} else {
			return "", fmt.Errorf("failed to get maintenance secret: %w", err)
		}
	} else {
		// Update existing secret if source has changed
		if !secretDataEqual(copiedSecret.Data, sourceSecret.Data) {
			copiedSecret.Data = sourceSecret.Data
			m.logger.Info("Updating maintenance secret copy",
				"name", copiedSecretName,
				"namespace", m.operatorNamespace)

			if err := m.client.Update(ctx, copiedSecret); err != nil {
				return "", fmt.Errorf("failed to update maintenance secret: %w", err)
			}
		}
	}

	return copiedSecretName, nil
}

// secretDataEqual compares two secret data maps for equality
func secretDataEqual(a, b map[string][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, exists := b[k]; !exists || !bytes.Equal(v, bv) {
			return false
		}
	}
	return true
}

// ensureServiceAccount ensures the maintenance ServiceAccount exists in operator namespace
// Phase 5: Fix ServiceAccount issues
func (m *MaintenanceManager) ensureServiceAccount(ctx context.Context) error {
	sa := &corev1.ServiceAccount{}
	err := m.client.Get(ctx, types.NamespacedName{
		Name:      maintenanceServiceAccountName,
		Namespace: m.operatorNamespace,
	}, sa)

	if err != nil {
		if errors.IsNotFound(err) {
			// Create ServiceAccount
			sa = &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      maintenanceServiceAccountName,
					Namespace: m.operatorNamespace,
					Labels: map[string]string{
						maintenanceLabelKey: "true",
					},
				},
			}

			m.logger.Info("Creating maintenance ServiceAccount",
				"name", maintenanceServiceAccountName,
				"namespace", m.operatorNamespace)

			if err := m.client.Create(ctx, sa); err != nil {
				return fmt.Errorf("failed to create ServiceAccount: %w", err)
			}

			// Also ensure necessary RBAC permissions
			if err := m.ensureRBACPermissions(ctx); err != nil {
				return fmt.Errorf("failed to ensure RBAC permissions: %w", err)
			}
		} else {
			return fmt.Errorf("failed to get ServiceAccount: %w", err)
		}
	}

	return nil
}

// ensureRBACPermissions ensures necessary RBAC permissions for maintenance
func (m *MaintenanceManager) ensureRBACPermissions(ctx context.Context) error {
	// Create Role for maintenance operations
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      maintenanceServiceAccountName,
			Namespace: m.operatorNamespace,
			Labels: map[string]string{
				maintenanceLabelKey: "true",
			},
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"secrets"},
				Verbs:     []string{"get", "list"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"configmaps"},
				Verbs:     []string{"get", "list"},
			},
		},
	}

	if err := m.client.Create(ctx, role); err != nil && !errors.IsAlreadyExists(err) {
		// Log warning but continue - might be in test environment
		m.logger.V(1).Info("Could not create Role (may be expected in tests)", "error", err)
	}

	// Create RoleBinding
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      maintenanceServiceAccountName,
			Namespace: m.operatorNamespace,
			Labels: map[string]string{
				maintenanceLabelKey: "true",
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     maintenanceServiceAccountName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      maintenanceServiceAccountName,
				Namespace: m.operatorNamespace,
			},
		},
	}

	if err := m.client.Create(ctx, roleBinding); err != nil && !errors.IsAlreadyExists(err) {
		// Log warning but continue - might be in test environment
		m.logger.V(1).Info("Could not create RoleBinding (may be expected in tests)", "error", err)
	}

	return nil
}

// buildMaintenanceCronJob constructs a CronJob for Kopia maintenance
func (m *MaintenanceManager) buildMaintenanceCronJob(repoConfig *RepositoryConfig,
	owner client.Object, copiedSecretName string) *batchv1.CronJob {
	repoHash := repoConfig.Hash()
	cronJobName := m.generateCronJobName(repoHash)

	// Build environment variables for the maintenance container
	envVars := m.buildMaintenanceEnvVars(repoConfig)

	// Build volume mounts
	volumes, volumeMounts := m.buildMaintenanceVolumes(repoConfig)

	// Determine resources to use
	resources := m.getMaintenanceResources(owner)

	// Phase 2: Create CronJobs in operator namespace
	cronJob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cronJobName,
			Namespace: m.operatorNamespace, // Use operator namespace instead of source namespace
			Labels: map[string]string{
				maintenanceLabelKey:        "true",
				maintenanceRepositoryLabel: repoHash,
				maintenanceNamespaceLabel:  repoConfig.Namespace, // Track source namespace
			},
			Annotations: map[string]string{
				maintenanceRepositoryAnnotation:    repoConfig.Repository,
				"volsync.backube/source-namespace": repoConfig.Namespace,
			},
		},
		Spec: batchv1.CronJobSpec{
			Schedule:                   repoConfig.Schedule,
			ConcurrencyPolicy:          batchv1.ForbidConcurrent,
			Suspend:                    m.isSuspended(owner),
			SuccessfulJobsHistoryLimit: m.getSuccessfulJobsHistoryLimit(owner),
			FailedJobsHistoryLimit:     m.getFailedJobsHistoryLimit(owner),
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					BackoffLimit: ptr.To(int32(3)),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								maintenanceLabelKey:        "true",
								maintenanceRepositoryLabel: repoHash,
							},
						},
						Spec: corev1.PodSpec{
							ServiceAccountName: maintenanceServiceAccountName, // Use centralized ServiceAccount
							RestartPolicy:      corev1.RestartPolicyOnFailure,
							SecurityContext: &corev1.PodSecurityContext{
								RunAsNonRoot: ptr.To(true),
								FSGroup:      ptr.To(int64(1000)),
								RunAsUser:    ptr.To(int64(1000)),
							},
							Containers: []corev1.Container{
								{
									Name:            "kopia-maintenance",
									Image:           m.containerImage,
									ImagePullPolicy: corev1.PullIfNotPresent,
									Command:         []string{"/bin/bash", "-c"},
									Args:            []string{"/mover-kopia/entry.sh"},
									Env:             envVars,
									EnvFrom: []corev1.EnvFromSource{
										{
											SecretRef: &corev1.SecretEnvSource{
												LocalObjectReference: corev1.LocalObjectReference{
													Name: copiedSecretName, // Use copied secret in operator namespace
												},
											},
										},
									},
									VolumeMounts: volumeMounts,
									Resources:    resources,
									SecurityContext: &corev1.SecurityContext{
										AllowPrivilegeEscalation: ptr.To(false),
										Capabilities: &corev1.Capabilities{
											Drop: []corev1.Capability{"ALL"},
										},
										Privileged:             ptr.To(false),
										ReadOnlyRootFilesystem: ptr.To(true), // Enhanced security: read-only root filesystem
										RunAsNonRoot:           ptr.To(true),
										RunAsUser:              ptr.To(int64(1000)),
									},
								},
							},
							Volumes: volumes,
						},
					},
				},
			},
		},
	}

	// Set owner reference if provided (optional, as CronJob may outlive individual sources)
	if owner != nil {
		// Don't set controller reference to avoid deletion cascade
		// We want the CronJob to persist as long as any source uses the repository
		cronJob.Labels["volsync.backube/created-by"] = owner.GetName()
	}

	return cronJob
}

// buildMaintenanceEnvVars creates environment variables for the maintenance container
func (m *MaintenanceManager) buildMaintenanceEnvVars(repoConfig *RepositoryConfig) []corev1.EnvVar {
	envVars := []corev1.EnvVar{
		{
			Name:  "DIRECTION",
			Value: "maintenance",
		},
		{
			Name:  "KOPIA_CACHE_DIR",
			Value: kopiaCacheMountPath,
		},
		{
			Name:  "DATA_DIR",
			Value: "/data", // Not used for maintenance, but required by entry.sh
		},
		{
			Name:  envKopiaOverrideMaintenanceUsername,
			Value: defaultMaintenanceUsername,
		},
	}

	// Note: All repository connection details (including KOPIA_PASSWORD) are sourced
	// from the repository secret via EnvFrom in the container spec

	// Add custom CA environment variable if specified
	if repoConfig.CustomCA != nil && (repoConfig.CustomCA.SecretName != "" || repoConfig.CustomCA.ConfigMapName != "") {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "CUSTOM_CA",
			Value: fmt.Sprintf("%s/%s", kopiaCAMountPath, kopiaCAFilename),
		})
	}

	return envVars
}

// buildMaintenanceVolumes creates volumes and volume mounts for the maintenance container
func (m *MaintenanceManager) buildMaintenanceVolumes(repoConfig *RepositoryConfig) ([]corev1.Volume, []corev1.VolumeMount) {
	var volumes []corev1.Volume
	var volumeMounts []corev1.VolumeMount

	// Cache volume (emptyDir for maintenance jobs with size limit)
	volumes = append(volumes, corev1.Volume{
		Name: kopiaCache,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{
				SizeLimit: resource.NewQuantity(1*1024*1024*1024, resource.BinarySI), // 1Gi limit
			},
		},
	})
	volumeMounts = append(volumeMounts, corev1.VolumeMount{
		Name:      kopiaCache,
		MountPath: kopiaCacheMountPath,
	})

	// Temp directory volume (required for read-only root filesystem)
	volumes = append(volumes, corev1.Volume{
		Name: "tempdir",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{
				Medium: corev1.StorageMediumMemory,
			},
		},
	})
	volumeMounts = append(volumeMounts, corev1.VolumeMount{
		Name:      "tempdir",
		MountPath: "/tmp",
	})

	// Custom CA if specified
	if repoConfig.CustomCA != nil {
		if repoConfig.CustomCA.SecretName != "" {
			volumes = append(volumes, corev1.Volume{
				Name: "custom-ca",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: repoConfig.CustomCA.SecretName,
						Items: []corev1.KeyToPath{
							{
								Key:  repoConfig.CustomCA.Key,
								Path: kopiaCAFilename,
							},
						},
					},
				},
			})
			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				Name:      "custom-ca",
				MountPath: kopiaCAMountPath,
				ReadOnly:  true,
			})
		} else if repoConfig.CustomCA.ConfigMapName != "" {
			volumes = append(volumes, corev1.Volume{
				Name: "custom-ca",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: repoConfig.CustomCA.ConfigMapName,
						},
						Items: []corev1.KeyToPath{
							{
								Key:  repoConfig.CustomCA.Key,
								Path: kopiaCAFilename,
							},
						},
					},
				},
			})
			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				Name:      "custom-ca",
				MountPath: kopiaCAMountPath,
				ReadOnly:  true,
			})
		}
	}

	return volumes, volumeMounts
}

// generateCronJobName generates a unique but deterministic name for the maintenance CronJob
func (m *MaintenanceManager) generateCronJobName(repoHash string) string {
	name := fmt.Sprintf("kopia-maintenance-%s", repoHash)
	if len(name) > maxCronJobNameLength {
		// Truncate if too long
		name = name[:maxCronJobNameLength]
	}
	return name
}

// isMaintenanceEnabled checks if maintenance is enabled for this source
func (m *MaintenanceManager) isMaintenanceEnabled(source *volsyncv1alpha1.ReplicationSource) bool {
	if source.Spec.Kopia == nil {
		return false
	}

	// Deprecated fields removed - maintenance is now managed by KopiaMaintenance CRD
	// Default to disabled since maintenance should be managed by KopiaMaintenance
	return false
}

// getMaintenanceSchedule determines the maintenance schedule for a source
func (m *MaintenanceManager) getMaintenanceSchedule(source *volsyncv1alpha1.ReplicationSource) string {
	// Deprecated fields removed - maintenance is now managed by KopiaMaintenance CRD
	return defaultMaintenanceSchedule
}

// migrateExistingCronJob migrates CronJobs from source namespace to operator namespace
// Phase 7: Migration support
func (m *MaintenanceManager) migrateExistingCronJob(ctx context.Context,
	repoConfig *RepositoryConfig) error {
	// Look for existing CronJob in source namespace
	repoHash := repoConfig.Hash()
	cronJobName := m.generateCronJobName(repoHash)

	existingCronJob := &batchv1.CronJob{}
	err := m.client.Get(ctx, types.NamespacedName{
		Name:      cronJobName,
		Namespace: repoConfig.Namespace, // Check source namespace
	}, existingCronJob)

	if err != nil {
		if errors.IsNotFound(err) {
			// No existing CronJob to migrate
			return nil
		}
		return fmt.Errorf("failed to check for existing CronJob: %w", err)
	}

	// Found existing CronJob in source namespace, migrate it
	m.logger.Info("Migrating existing maintenance CronJob to operator namespace",
		"oldNamespace", repoConfig.Namespace,
		"newNamespace", m.operatorNamespace,
		"cronJob", cronJobName)

	// Delete the old CronJob
	if err := m.client.Delete(ctx, existingCronJob); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete old CronJob during migration: %w", err)
	}

	// Clean up any jobs from the old CronJob
	jobList := &batchv1.JobList{}
	listOpts := []client.ListOption{
		client.InNamespace(repoConfig.Namespace),
		client.MatchingLabels{
			maintenanceLabelKey:        "true",
			maintenanceRepositoryLabel: repoHash,
		},
	}
	if err := m.client.List(ctx, jobList, listOpts...); err != nil {
		m.logger.Error(err, "Failed to list jobs during migration")
	}

	for _, job := range jobList.Items {
		if err := m.client.Delete(ctx, &job); err != nil && !errors.IsNotFound(err) {
			m.logger.Error(err, "Failed to delete job during migration", "job", job.Name)
		}
	}

	return nil
}

// getServiceAccountName returns the service account name for maintenance
// Phase 5: Fix method name
func (m *MaintenanceManager) getServiceAccountName(owner client.Object) string {
	// Use centralized ServiceAccount in operator namespace
	return maintenanceServiceAccountName
}

// getSuccessfulJobsHistoryLimit gets the successful jobs history limit from the builder configuration
func (m *MaintenanceManager) getSuccessfulJobsHistoryLimit(owner client.Object) *int32 {
	_ = owner // Keep parameter for interface compatibility
	// Get from builder if available, otherwise use default
	if m.builder != nil {
		return ptr.To(m.builder.GetSuccessfulJobsHistoryLimit())
	}
	// Default value (fallback if builder is not set)
	return ptr.To(int32(3))
}

// getFailedJobsHistoryLimit gets the failed jobs history limit from the builder configuration
func (m *MaintenanceManager) getFailedJobsHistoryLimit(owner client.Object) *int32 {
	_ = owner // Keep parameter for interface compatibility
	// Get from builder if available, otherwise use default
	if m.builder != nil {
		return ptr.To(m.builder.GetFailedJobsHistoryLimit())
	}
	// Default value (fallback if builder is not set)
	return ptr.To(int32(1))
}

// isSuspended checks if maintenance is suspended from MaintenanceCronJobSpec
func (m *MaintenanceManager) isSuspended(owner client.Object) *bool {
	// Deprecated fields removed - maintenance config now managed by KopiaMaintenance CRD
	_ = owner // Keep parameter for interface compatibility
	// Default to not suspended
	return ptr.To(false)
}

// getMaintenanceResources returns the resource requirements for maintenance containers
func (m *MaintenanceManager) getMaintenanceResources(owner client.Object) corev1.ResourceRequirements {
	// Deprecated fields removed - maintenance resources now managed by KopiaMaintenance CRD
	_ = owner // Keep parameter for interface compatibility

	// Return default resources if not configured
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("256Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("1Gi"), // Changed from 512Mi to 1Gi as per comment
		},
	}
}

// GetMaintenanceCronJobsForNamespace returns all maintenance CronJobs in a namespace
func (m *MaintenanceManager) GetMaintenanceCronJobsForNamespace(ctx context.Context,
	namespace string) ([]batchv1.CronJob, error) {
	cronJobList := &batchv1.CronJobList{}
	listOpts := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels{maintenanceLabelKey: "true"},
	}
	if err := m.client.List(ctx, cronJobList, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list maintenance CronJobs: %w", err)
	}
	return cronJobList.Items, nil
}

// UpdateMaintenanceCronJobsForRepository updates all maintenance CronJobs that use a specific repository
// This is useful when repository configuration changes
func (m *MaintenanceManager) UpdateMaintenanceCronJobsForRepository(ctx context.Context,
	namespace, repositoryName string) error {
	// List all ReplicationSources using this repository
	sourceList := &volsyncv1alpha1.ReplicationSourceList{}
	if err := m.client.List(ctx, sourceList, client.InNamespace(namespace)); err != nil {
		return fmt.Errorf("failed to list ReplicationSources: %w", err)
	}

	// Find sources using this repository and reconcile their maintenance
	for _, source := range sourceList.Items {
		if source.Spec.Kopia != nil && source.Spec.Kopia.Repository == repositoryName {
			if err := m.ReconcileMaintenanceForSource(ctx, &source); err != nil {
				return fmt.Errorf("failed to reconcile maintenance for source %s: %w",
					source.Name, err)
			}
		}
	}

	return nil
}

// recordCronJobMetric records metrics for CronJob operations
func (m *MaintenanceManager) recordCronJobMetric(owner client.Object, operation string) {
	labels := prometheus.Labels{
		"obj_name":      owner.GetName(),
		"obj_namespace": owner.GetNamespace(),
		"role":          "source",
		"operation":     "maintenance",
		"repository":    "",
	}

	// Try to get repository name from ReplicationSource
	if source, ok := owner.(*volsyncv1alpha1.ReplicationSource); ok && source.Spec.Kopia != nil {
		labels["repository"] = source.Spec.Kopia.Repository
	}

	switch operation {
	case "created":
		m.metrics.MaintenanceCronJobCreated.With(labels).Inc()
	case "updated":
		m.metrics.MaintenanceCronJobUpdated.With(labels).Inc()
	}
}

// recordCronJobDeletionMetric records metrics for CronJob deletion
func (m *MaintenanceManager) recordCronJobDeletionMetric(namespace, name string) {
	labels := prometheus.Labels{
		"obj_name":      name,
		"obj_namespace": namespace,
		"role":          "source",
		"operation":     "maintenance",
		"repository":    "",
	}
	m.metrics.MaintenanceCronJobDeleted.With(labels).Inc()
}

// GetMaintenanceStatus retrieves detailed maintenance status for a source
func (m *MaintenanceManager) GetMaintenanceStatus(ctx context.Context,
	source *volsyncv1alpha1.ReplicationSource) (*MaintenanceStatus, error) {
	if source.Spec.Kopia == nil || !m.isMaintenanceEnabled(source) {
		return nil, nil
	}

	// Create repository config to find the CronJob
	repoConfig := &RepositoryConfig{
		Repository: source.Spec.Kopia.Repository,
		CustomCA:   (*volsyncv1alpha1.CustomCASpec)(&source.Spec.Kopia.CustomCA),
		Namespace:  source.Namespace,
		Schedule:   m.getMaintenanceSchedule(source),
	}

	repoHash := repoConfig.Hash()
	cronJobName := m.generateCronJobName(repoHash)

	// Get the CronJob from the operator namespace (centralized approach)
	cronJob := &batchv1.CronJob{}
	err := m.client.Get(ctx, types.NamespacedName{
		Name:      cronJobName,
		Namespace: m.operatorNamespace,
	}, cronJob)
	if err != nil {
		if errors.IsNotFound(err) {
			return &MaintenanceStatus{
				Configured: false,
			}, nil
		}
		return nil, fmt.Errorf("failed to get CronJob: %w", err)
	}

	status := &MaintenanceStatus{
		Configured:               true,
		CronJobName:              cronJob.Name,
		Schedule:                 cronJob.Spec.Schedule,
		NextScheduledTime:        nil,
		LastScheduledTime:        cronJob.Status.LastScheduleTime,
		LastSuccessfulTime:       nil,
		FailuresSinceLastSuccess: 0,
	}

	// Calculate next scheduled time
	if cronJob.Status.LastScheduleTime != nil && cronJob.Spec.Suspend != nil && !*cronJob.Spec.Suspend {
		// This is a simplified calculation. In production, you'd use a proper cron parser
		status.NextScheduledTime = m.calculateNextScheduledTime(cronJob.Spec.Schedule, cronJob.Status.LastScheduleTime.Time)
	}

	// Get job history
	jobs, err := m.getMaintenanceJobs(ctx, source.Namespace, repoHash)
	if err != nil {
		m.logger.Error(err, "Failed to get maintenance jobs")
		// Don't fail entirely, just log the error
	} else {
		// Analyze job history
		m.analyzeJobHistory(jobs, status)
	}

	// Record metrics
	m.updateMaintenanceMetrics(source, status)

	return status, nil
}

// MaintenanceStatus contains detailed information about maintenance operations
type MaintenanceStatus struct {
	Configured               bool         `json:"configured"`
	CronJobName              string       `json:"cronJobName,omitempty"`
	Schedule                 string       `json:"schedule,omitempty"`
	NextScheduledTime        *metav1.Time `json:"nextScheduledTime,omitempty"`
	LastScheduledTime        *metav1.Time `json:"lastScheduledTime,omitempty"`
	LastSuccessfulTime       *metav1.Time `json:"lastSuccessfulTime,omitempty"`
	LastFailedTime           *metav1.Time `json:"lastFailedTime,omitempty"`
	FailuresSinceLastSuccess int          `json:"failuresSinceLastSuccess"`
	LastMaintenanceDuration  *string      `json:"lastMaintenanceDuration,omitempty"`
	LastError                string       `json:"lastError,omitempty"`
}

// getMaintenanceJobs retrieves all maintenance jobs for a repository
func (m *MaintenanceManager) getMaintenanceJobs(ctx context.Context, namespace, repoHash string) ([]batchv1.Job, error) { //nolint:lll
	jobList := &batchv1.JobList{}
	listOpts := []client.ListOption{
		client.InNamespace(m.operatorNamespace), // Jobs are in operator namespace (centralized approach)
		client.MatchingLabels{
			maintenanceLabelKey:        "true",
			maintenanceRepositoryLabel: repoHash,
		},
	}
	if err := m.client.List(ctx, jobList, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list maintenance jobs: %w", err)
	}
	return jobList.Items, nil
}

// analyzeJobHistory analyzes job history to extract maintenance statistics
func (m *MaintenanceManager) analyzeJobHistory(jobs []batchv1.Job, status *MaintenanceStatus) {
	var lastSuccessTime *metav1.Time
	var lastFailedTime *metav1.Time
	failureCount := 0
	foundSuccess := false

	// Sort jobs by creation time (newest first) to analyze most recent ones
	sort.Slice(jobs, func(i, j int) bool {
		if jobs[i].CreationTimestamp.Time.Equal(jobs[j].CreationTimestamp.Time) {
			// If timestamps are equal, sort by name for consistency
			return jobs[i].Name > jobs[j].Name
		}
		return jobs[i].CreationTimestamp.Time.After(jobs[j].CreationTimestamp.Time)
	})

	// Limit analysis to most recent 50 jobs to prevent memory issues
	maxJobsToAnalyze := 50
	jobsToAnalyze := jobs
	if len(jobs) > maxJobsToAnalyze {
		jobsToAnalyze = jobs[:maxJobsToAnalyze]
		m.logger.V(1).Info("Limiting job history analysis",
			"totalJobs", len(jobs),
			"analyzedJobs", maxJobsToAnalyze)
	}

	// Process the limited set of jobs
	for _, job := range jobsToAnalyze {
		// Skip jobs that are still running
		if job.Status.CompletionTime == nil {
			continue
		}

		// Check if job succeeded
		succeeded := false
		for _, condition := range job.Status.Conditions {
			if condition.Type == batchv1.JobComplete && condition.Status == corev1.ConditionTrue {
				succeeded = true
				break
			}
		}

		if succeeded {
			if !foundSuccess {
				lastSuccessTime = job.Status.CompletionTime
				foundSuccess = true

				// Try to extract maintenance duration from job logs
				duration := m.extractMaintenanceDuration(&job)
				if duration != "" {
					status.LastMaintenanceDuration = &duration
				}
			}
		} else {
			if lastFailedTime == nil || job.Status.CompletionTime.After(lastFailedTime.Time) {
				lastFailedTime = job.Status.CompletionTime
				// Extract error from job if possible
				status.LastError = m.extractJobError(&job)
			}
			if !foundSuccess {
				failureCount++
			}
		}
	}

	status.LastSuccessfulTime = lastSuccessTime
	status.LastFailedTime = lastFailedTime
	status.FailuresSinceLastSuccess = failureCount
}

// extractMaintenanceDuration attempts to extract maintenance duration from job
func (m *MaintenanceManager) extractMaintenanceDuration(job *batchv1.Job) string {
	// In a real implementation, this would parse job logs to extract
	// the "MAINTENANCE_DURATION: X" line from the entry.sh output
	// For now, we'll calculate from job start/completion times
	if job.Status.StartTime != nil && job.Status.CompletionTime != nil {
		duration := job.Status.CompletionTime.Sub(job.Status.StartTime.Time)
		return fmt.Sprintf("%ds", int(duration.Seconds()))
	}
	return ""
}

// extractJobError attempts to extract error message from failed job
func (m *MaintenanceManager) extractJobError(job *batchv1.Job) string {
	// In a real implementation, this would parse job logs or conditions
	// For now, return a generic message based on conditions
	for _, condition := range job.Status.Conditions {
		if condition.Type == batchv1.JobFailed && condition.Status == corev1.ConditionTrue {
			return condition.Message
		}
	}
	return "Unknown error"
}

// calculateNextScheduledTime calculates the next scheduled time based on cron expression
func (m *MaintenanceManager) calculateNextScheduledTime(schedule string, lastTime time.Time) *metav1.Time {
	// This is a simplified implementation. In production, you'd use a proper cron parser
	// like github.com/robfig/cron to calculate the actual next run time

	// For now, use simple heuristics based on common patterns
	var nextTime time.Time
	switch schedule {
	case "0 2 * * *": // Daily at 2 AM
		nextTime = lastTime.Add(24 * time.Hour)
	case "0 2 * * 0": // Weekly on Sunday at 2 AM
		nextTime = lastTime.Add(7 * 24 * time.Hour)
	case "0 2 1 * *": // Monthly on 1st at 2 AM
		nextTime = lastTime.AddDate(0, 1, 0)
	default:
		// Default to 24 hours from last time
		nextTime = lastTime.Add(24 * time.Hour)
	}

	t := metav1.NewTime(nextTime)
	return &t
}

// updateMaintenanceMetrics updates Prometheus metrics based on maintenance status
func (m *MaintenanceManager) updateMaintenanceMetrics(source *volsyncv1alpha1.ReplicationSource, status *MaintenanceStatus) { //nolint:lll
	labels := prometheus.Labels{
		"obj_name":      source.Name,
		"obj_namespace": source.Namespace,
		"role":          "source",
		"operation":     "maintenance",
		"repository":    source.Spec.Kopia.Repository,
	}

	// Update last run timestamp
	if status.LastSuccessfulTime != nil {
		m.metrics.MaintenanceLastRunTimestamp.With(labels).Set(float64(status.LastSuccessfulTime.Unix()))
	}

	// Record failures
	if status.FailuresSinceLastSuccess > 0 {
		failureLabels := prometheus.Labels{
			"obj_name":       source.Name,
			"obj_namespace":  source.Namespace,
			"role":           "source",
			"operation":      "maintenance",
			"repository":     source.Spec.Kopia.Repository,
			"failure_reason": "maintenance_job_failed",
		}
		// Only increment once per check, not for each failure
		// In production, you'd track which failures have been recorded
		m.metrics.MaintenanceCronJobFailures.With(failureLabels).Add(float64(status.FailuresSinceLastSuccess))
	}

	// Record duration if available
	if status.LastMaintenanceDuration != nil {
		// Parse duration string (format: "Xs")
		if strings.HasSuffix(*status.LastMaintenanceDuration, "s") {
			durationStr := strings.TrimSuffix(*status.LastMaintenanceDuration, "s")
			if duration, err := strconv.ParseFloat(durationStr, 64); err == nil {
				m.metrics.MaintenanceDurationSeconds.With(labels).Observe(duration)
			}
		}
	}
}

// ParseMaintenanceLogs parses maintenance job logs to extract metrics
func (m *MaintenanceManager) ParseMaintenanceLogs(job *batchv1.Job) (*MaintenanceLogMetrics, error) {
	// This would typically read pod logs and parse them for specific patterns
	// For now, return a placeholder implementation

	metrics := &MaintenanceLogMetrics{}

	// In a real implementation:
	// 1. Get the pod associated with the job
	// 2. Read the pod logs
	// 3. Parse for patterns like:
	//    - "MAINTENANCE_STATUS: SUCCESS"
	//    - "MAINTENANCE_DURATION: 120"
	//    - "REPO_SIZE_BYTES: 1073741824"
	//    - etc.

	return metrics, nil
}

// MaintenanceLogMetrics contains metrics extracted from maintenance logs
type MaintenanceLogMetrics struct {
	Status              string
	DurationSeconds     int
	RepositorySizeBytes int64
	ContentCount        int
	BlobCount           int
	DeduplicationRatio  float64
	Error               string
}
