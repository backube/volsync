/*
Copyright 2025 The VolSync authors.

This file may be used, at your option, according to either the GNU AGPL 3.0 or
the Apache V2 license.

---
This program is free software: you can redistribute it and/or modify it under
the terms of the GNU Affero General Public License as published by the Free
Software Foundation, either version 3 of the License, or (at your option) any
later version.

This program is distributed in the hope that it will be useful, but WITHOUT ANY
WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR A
PARTICULAR PURPOSE.  See the GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License along
with this program.  If not, see <https://www.gnu.org/licenses/>.

---
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// +kubebuilder:validation:Required
package v1alpha1

import (
	"fmt"
	"strings"

	cron "github.com/robfig/cron/v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KopiaMaintenanceTriggerSpec defines when maintenance will be performed.
type KopiaMaintenanceTriggerSpec struct {
	// schedule is a cronspec (https://en.wikipedia.org/wiki/Cron#Overview) that
	// can be used to schedule maintenance to occur at regular, time-based
	// intervals.
	// nolint:lll
	//+kubebuilder:validation:Pattern=`^(@(annually|yearly|monthly|weekly|daily|hourly))|((((\d+,)*\d+|(\d+(\/|-)\d+)|\*(\/\d+)?)\s?){5})$`
	//+optional
	Schedule *string `json:"schedule,omitempty"`
	// manual is a string value that schedules a manual trigger.
	// Once a maintenance completes then status.lastManualSync is set to the same string value.
	// A consumer of a manual trigger should set spec.trigger.manual to a known value
	// and then wait for lastManualSync to be updated by the operator to the same value,
	// which means that the manual trigger will then pause and wait for further
	// updates to the trigger.
	//+optional
	Manual string `json:"manual,omitempty"`
}

// KopiaMaintenanceSpec defines the desired state of KopiaMaintenance
type KopiaMaintenanceSpec struct {
	// Repository defines the repository configuration for maintenance.
	// The repository secret must exist in the same namespace as the KopiaMaintenance resource.
	// +kubebuilder:validation:Required
	Repository KopiaRepositorySpec `json:"repository"`

	// Trigger determines when maintenance will be performed.
	// If not specified, defaults to a schedule of "0 2 * * *" (daily at 2am).
	//+optional
	Trigger *KopiaMaintenanceTriggerSpec `json:"trigger,omitempty"`

	// Schedule is a cron schedule for when maintenance should run.
	// The schedule is interpreted in the controller's timezone.
	// DEPRECATED: Use Trigger.Schedule instead. This field will be removed in a future version.
	// +kubebuilder:validation:Pattern=`^(@(annually|yearly|monthly|weekly|daily|hourly))|((((\d+,)*\d+|(\d+(\/|-)\d+)|\*(\/\d+)?)\s?){5})$`
	// +kubebuilder:default="0 2 * * *"
	// +optional
	// +deprecated
	Schedule string `json:"schedule,omitempty"`

	// Enabled determines if maintenance should be performed.
	// When false, no maintenance will be scheduled.
	// +kubebuilder:default=true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Suspend can be used to temporarily stop maintenance. When true,
	// the CronJob will not create new Jobs, but existing Jobs will be allowed
	// to complete.
	// +optional
	Suspend *bool `json:"suspend,omitempty"`

	// SuccessfulJobsHistoryLimit specifies how many successful maintenance Jobs
	// should be kept. Defaults to 3.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=3
	// +optional
	SuccessfulJobsHistoryLimit *int32 `json:"successfulJobsHistoryLimit,omitempty"`

	// FailedJobsHistoryLimit specifies how many failed maintenance Jobs
	// should be kept. Defaults to 1.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=1
	// +optional
	FailedJobsHistoryLimit *int32 `json:"failedJobsHistoryLimit,omitempty"`

	// Resources represents compute resources required by the maintenance container.
	// If not specified, defaults to 256Mi memory request and 1Gi memory limit.
	// More info: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// ActiveDeadlineSeconds specifies the duration in seconds relative to the startTime
	// that the job may be active before the system tries to terminate it.
	// If not specified, defaults to 10800 (3 hours).
	// This prevents maintenance jobs from running indefinitely.
	// For repositories requiring longer maintenance windows, increase this value.
	// +kubebuilder:validation:Minimum=600
	// +kubebuilder:default=10800
	// +optional
	ActiveDeadlineSeconds *int64 `json:"activeDeadlineSeconds,omitempty"`

	// ServiceAccountName allows specifying a custom ServiceAccount for maintenance jobs.
	// If not specified, a default maintenance ServiceAccount will be used.
	// +optional
	ServiceAccountName *string `json:"serviceAccountName,omitempty"`

	// PodSecurityContext defines the security context for maintenance pods.
	// This allows configuring pod-level security settings such as runAsUser, fsGroup, etc.
	// These settings are inherited by the container automatically.
	// If not specified, defaults to runAsUser: 1000, fsGroup: 1000, runAsNonRoot: true.
	//
	// For most users, setting runAsUser here is sufficient - the container will inherit it.
	// +optional
	PodSecurityContext *corev1.PodSecurityContext `json:"podSecurityContext,omitempty"`

	// ContainerSecurityContext defines the security context for the maintenance container.
	// This is for advanced use cases where you need to override specific container-level
	// security settings (capabilities, privileged, seLinux, seccomp, etc.).
	//
	// IMPORTANT: For setting the user ID, use PodSecurityContext.runAsUser instead.
	// The container automatically inherits runAsUser from the pod-level context.
	//
	// If not specified, defaults to security hardening settings:
	// - readOnlyRootFilesystem: true
	// - allowPrivilegeEscalation: false
	// - capabilities.drop: ["ALL"]
	// - runAsNonRoot: true
	//
	// Example advanced usage (dropping specific capabilities):
	//   containerSecurityContext:
	//     capabilities:
	//       drop: ["NET_RAW", "SYS_CHROOT"]
	// +optional
	ContainerSecurityContext *corev1.SecurityContext `json:"containerSecurityContext,omitempty"`

	// MoverPodLabels that should be added to maintenance pods.
	// These will be in addition to any labels that VolSync may add.
	// +optional
	MoverPodLabels map[string]string `json:"moverPodLabels,omitempty"`

	// NodeSelector for maintenance pods.
	// Use to schedule maintenance jobs on specific nodes.
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Tolerations for maintenance pods.
	// Allows scheduling on nodes with matching taints.
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Affinity for maintenance pods.
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// CacheCapacity can be used to set the size of the Kopia metadata cache volume
	// +optional
	CacheCapacity *resource.Quantity `json:"cacheCapacity,omitempty"`

	// CacheStorageClassName can be used to set the StorageClass of the Kopia metadata cache volume
	// +optional
	CacheStorageClassName *string `json:"cacheStorageClassName,omitempty"`

	// CacheAccessModes can be used to set the accessModes of Kopia metadata cache volume
	//+optional
	CacheAccessModes []corev1.PersistentVolumeAccessMode `json:"cacheAccessModes,omitempty"`
	// MetadataCacheSizeLimitMB is the hard limit for Kopia's metadata cache in MB.
	// If not specified, auto-calculated as 70% of CacheCapacity.
	// Set to 0 for unlimited (Kopia default behavior).
	//+optional
	MetadataCacheSizeLimitMB *int32 `json:"metadataCacheSizeLimitMB,omitempty"`
	// ContentCacheSizeLimitMB is the hard limit for Kopia's content cache in MB.
	// If not specified, auto-calculated as 20% of CacheCapacity.
	// Set to 0 for unlimited (Kopia default behavior).
	//+optional
	ContentCacheSizeLimitMB *int32 `json:"contentCacheSizeLimitMB,omitempty"`

	// CachePVC is the name of an existing PVC to use for Kopia cache. If not specified,
	// cache will be determined by other cache fields or use EmptyDir as fallback.
	//+optional
	CachePVC *string `json:"cachePVC,omitempty"`
}

// KopiaRepositorySpec defines the repository configuration for maintenance
type KopiaRepositorySpec struct {
	// Repository is the secret name containing repository configuration.
	// This secret should contain the repository connection details (URL, credentials, etc.)
	// in the same format as used by ReplicationSources.
	// The secret must exist in the same namespace as the KopiaMaintenance resource.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Repository string `json:"repository"`

	// CustomCA is optional custom CA configuration for repository access.
	// +optional
	CustomCA *ReplicationSourceKopiaCA `json:"customCA,omitempty"`

	// RepositoryType specifies the type of repository (e.g., "s3", "azure", "gcs", "filesystem").
	// This helps with validation and provides metadata for maintenance operations.
	// +optional
	RepositoryType string `json:"repositoryType,omitempty"`
}


// KopiaMaintenanceStatus defines the observed state of KopiaMaintenance
type KopiaMaintenanceStatus struct {
	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// ActiveCronJob is the name of the CronJob currently managed by this maintenance configuration.
	// +optional
	ActiveCronJob string `json:"activeCronJob,omitempty"`

	// LastReconcileTime is the last time this maintenance configuration was reconciled.
	// +optional
	LastReconcileTime *metav1.Time `json:"lastReconcileTime,omitempty"`

	// LastMaintenanceTime is the last time maintenance was successfully performed.
	// +optional
	LastMaintenanceTime *metav1.Time `json:"lastMaintenanceTime,omitempty"`

	// NextScheduledMaintenance is the next scheduled maintenance time.
	// +optional
	NextScheduledMaintenance *metav1.Time `json:"nextScheduledMaintenance,omitempty"`

	// MaintenanceFailures counts the number of consecutive maintenance failures.
	// +optional
	MaintenanceFailures int32 `json:"maintenanceFailures,omitempty"`

	// LastManualSync is set to the last spec.trigger.manual when the manual maintenance is done.
	// +optional
	LastManualSync string `json:"lastManualSync,omitempty"`

	// Conditions represent the latest available observations of the
	// maintenance configuration's state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// KopiaMaintenance is a VolSync resource that defines maintenance configuration
// for Kopia repositories. It manages repository maintenance operations
// on a defined schedule.
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Repository",type="string",JSONPath=`.spec.repository.repository`
// +kubebuilder:printcolumn:name="Enabled",type="boolean",JSONPath=`.spec.enabled`
// +kubebuilder:printcolumn:name="Schedule",type="string",JSONPath=`.spec.schedule`
// +kubebuilder:printcolumn:name="Suspended",type="boolean",JSONPath=`.spec.suspend`
// +kubebuilder:printcolumn:name="Last Maintenance",type="string",format="date-time",JSONPath=`.status.lastMaintenanceTime`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=`.metadata.creationTimestamp`
type KopiaMaintenance struct {
	metav1.TypeMeta `json:",inline"`
	//+optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// spec is the desired state of the KopiaMaintenance, including the
	// repository matching criteria and maintenance configuration.
	Spec KopiaMaintenanceSpec `json:"spec,omitempty"`
	// status is the observed state of the KopiaMaintenance as determined by
	// the controller.
	//+optional
	Status *KopiaMaintenanceStatus `json:"status,omitempty"`
}

// KopiaMaintenanceList contains a list of KopiaMaintenance
// +kubebuilder:object:root=true
type KopiaMaintenanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KopiaMaintenance `json:"items"`
}

// GetEnabled returns whether maintenance is enabled
func (km *KopiaMaintenance) GetEnabled() bool {
	if km.Spec.Enabled == nil {
		return true // Default to enabled
	}
	return *km.Spec.Enabled
}

// GetSchedule returns the maintenance schedule
func (km *KopiaMaintenance) GetSchedule() string {
	// Check new trigger field first
	if km.Spec.Trigger != nil && km.Spec.Trigger.Schedule != nil {
		return *km.Spec.Trigger.Schedule
	}
	// Fall back to deprecated Schedule field
	if km.Spec.Schedule != "" {
		return km.Spec.Schedule
	}
	return "0 2 * * *" // Default schedule
}

// GetManualTrigger returns the manual trigger value if set
func (km *KopiaMaintenance) GetManualTrigger() string {
	if km.Spec.Trigger != nil {
		return km.Spec.Trigger.Manual
	}
	return ""
}

// HasScheduleTrigger returns true if this resource uses schedule-based triggering
func (km *KopiaMaintenance) HasScheduleTrigger() bool {
	// Has schedule if either new trigger.schedule or deprecated schedule field is set
	return (km.Spec.Trigger != nil && km.Spec.Trigger.Schedule != nil) || km.Spec.Schedule != ""
}

// HasManualTrigger returns true if this resource uses manual triggering
func (km *KopiaMaintenance) HasManualTrigger() bool {
	return km.Spec.Trigger != nil && km.Spec.Trigger.Manual != ""
}

// GetRepositorySecret returns the repository secret name
func (km *KopiaMaintenance) GetRepositorySecret() string {
	return km.Spec.Repository.Repository
}

// GetActiveDeadlineSeconds returns the job timeout in seconds
func (km *KopiaMaintenance) GetActiveDeadlineSeconds() int64 {
	if km.Spec.ActiveDeadlineSeconds != nil {
		return *km.Spec.ActiveDeadlineSeconds
	}
	// Default to 3 hours (10800 seconds)
	return 10800
}

// Validate validates the KopiaMaintenance configuration
func (km *KopiaMaintenance) Validate() error {
	if km.Spec.Repository.Repository == "" {
		return fmt.Errorf("repository.repository field is required")
	}

	// Validate trigger configuration
	if km.Spec.Trigger != nil {
		// Can't have both schedule and manual triggers
		if km.Spec.Trigger.Schedule != nil && km.Spec.Trigger.Manual != "" {
			return fmt.Errorf("cannot specify both schedule and manual triggers")
		}

		// Validate schedule format if present
		if km.Spec.Trigger.Schedule != nil && *km.Spec.Trigger.Schedule != "" {
			parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
			if _, err := parser.Parse(*km.Spec.Trigger.Schedule); err != nil {
				return fmt.Errorf("invalid cron schedule format in trigger: %w", err)
			}
		}
	}

	// Validate deprecated cron schedule format
	if km.Spec.Schedule != "" {
		// Parse the cron schedule to validate format
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		if _, err := parser.Parse(km.Spec.Schedule); err != nil {
			return fmt.Errorf("invalid cron schedule format: %w", err)
		}
	}

	// Warn if both new and deprecated fields are used
	if km.Spec.Trigger != nil && km.Spec.Trigger.Schedule != nil && km.Spec.Schedule != "" {
		// This is not an error, but the new field takes precedence
		// Controller should log a warning about this
	}

	// Validate resource requirements if specified
	if km.Spec.Resources != nil {
		if err := validateResourceRequirements(km.Spec.Resources); err != nil {
			return fmt.Errorf("invalid resource requirements: %w", err)
		}
	}

	return nil
}

// validateResourceRequirements validates resource requirements
func validateResourceRequirements(resources *corev1.ResourceRequirements) error {
	// Check memory limits and requests
	if resources.Limits != nil {
		if memLimit, ok := resources.Limits[corev1.ResourceMemory]; ok {
			if memLimit.Sign() <= 0 {
				return fmt.Errorf("memory limit must be positive")
			}
			// Check for reasonable maximum (e.g., 100Gi)
			maxMemory := resource.MustParse("100Gi")
			if memLimit.Cmp(maxMemory) > 0 {
				return fmt.Errorf("memory limit exceeds maximum allowed (100Gi)")
			}
		}
		if cpuLimit, ok := resources.Limits[corev1.ResourceCPU]; ok {
			if cpuLimit.Sign() <= 0 {
				return fmt.Errorf("CPU limit must be positive")
			}
			// Check for reasonable maximum (e.g., 32 cores)
			maxCPU := resource.MustParse("32")
			if cpuLimit.Cmp(maxCPU) > 0 {
				return fmt.Errorf("CPU limit exceeds maximum allowed (32 cores)")
			}
		}
	}

	if resources.Requests != nil {
		if memRequest, ok := resources.Requests[corev1.ResourceMemory]; ok {
			if memRequest.Sign() <= 0 {
				return fmt.Errorf("memory request must be positive")
			}
			// If limit is set, request must not exceed limit
			if resources.Limits != nil {
				if memLimit, hasLimit := resources.Limits[corev1.ResourceMemory]; hasLimit {
					if memRequest.Cmp(memLimit) > 0 {
						return fmt.Errorf("memory request exceeds memory limit")
					}
				}
			}
		}
		if cpuRequest, ok := resources.Requests[corev1.ResourceCPU]; ok {
			if cpuRequest.Sign() <= 0 {
				return fmt.Errorf("CPU request must be positive")
			}
			// If limit is set, request must not exceed limit
			if resources.Limits != nil {
				if cpuLimit, hasLimit := resources.Limits[corev1.ResourceCPU]; hasLimit {
					if cpuRequest.Cmp(cpuLimit) > 0 {
						return fmt.Errorf("CPU request exceeds CPU limit")
					}
				}
			}
		}
	}

	// Validate that only standard resources are specified
	for resourceName := range resources.Limits {
		if !isStandardResourceName(string(resourceName)) {
			return fmt.Errorf("unsupported resource limit: %s", resourceName)
		}
	}
	for resourceName := range resources.Requests {
		if !isStandardResourceName(string(resourceName)) {
			return fmt.Errorf("unsupported resource request: %s", resourceName)
		}
	}

	return nil
}

// isStandardResourceName checks if the resource name is a standard Kubernetes resource
func isStandardResourceName(name string) bool {
	standardResources := []string{
		string(corev1.ResourceCPU),
		string(corev1.ResourceMemory),
		string(corev1.ResourceEphemeralStorage),
	}
	for _, std := range standardResources {
		if name == std {
			return true
		}
	}
	// Also allow hugepages resources
	if strings.HasPrefix(name, "hugepages-") {
		return true
	}
	return false
}

func init() {
	SchemeBuilder.Register(&KopiaMaintenance{}, &KopiaMaintenanceList{})
}