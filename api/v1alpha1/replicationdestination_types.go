/*
Copyright 2020 The VolSync authors.

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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ReplicationDestinationTriggerSpec defines when a volume will be synchronized
// with the source.
type ReplicationDestinationTriggerSpec struct {
	// schedule is a cronspec (https://en.wikipedia.org/wiki/Cron#Overview) that
	// can be used to schedule replication to occur at regular, time-based
	// intervals.
	//+kubebuilder:validation:Pattern=`^(\d+|\*)(/\d+)?(\s+(\d+|\*)(/\d+)?){4}$`
	//+optional
	Schedule *string `json:"schedule,omitempty"`
	// manual is a string value that schedules a manual trigger.
	// Once a sync completes then status.lastManualSync is set to the same string value.
	// A consumer of a manual trigger should set spec.trigger.manual to a known value
	// and then wait for lastManualSync to be updated by the operator to the same value,
	// which means that the manual trigger will then pause and wait for further
	// updates to the trigger.
	//+optional
	Manual string `json:"manual,omitempty"`
}

type ReplicationDestinationVolumeOptions struct {
	// copyMethod describes how a point-in-time (PiT) image of the destination
	// volume should be created.
	CopyMethod CopyMethodType `json:"copyMethod,omitempty"`
	// capacity is the size of the destination volume to create.
	//+optional
	Capacity *resource.Quantity `json:"capacity,omitempty"`
	// storageClassName can be used to specify the StorageClass of the
	// destination volume. If not set, the default StorageClass will be used.
	//+optional
	StorageClassName *string `json:"storageClassName,omitempty"`
	// accessModes specifies the access modes for the destination volume.
	//+kubebuilder:validation:MinItems=1
	//+optional
	AccessModes []corev1.PersistentVolumeAccessMode `json:"accessModes,omitempty"`
	// volumeSnapshotClassName can be used to specify the VSC to be used if
	// copyMethod is Snapshot. If not set, the default VSC is used.
	//+optional
	VolumeSnapshotClassName *string `json:"volumeSnapshotClassName,omitempty"`
	// destinationPVC is a PVC to use as the transfer destination instead of
	// automatically provisioning one. Either this field or both capacity and
	// accessModes must be specified.
	//+optional
	DestinationPVC *string `json:"destinationPVC,omitempty"`
}

type ReplicationDestinationRsyncSpec struct {
	ReplicationDestinationVolumeOptions `json:",inline"`
	// sshKeys is the name of a Secret that contains the SSH keys to be used for
	// authentication. If not provided, the keys will be generated.
	//+optional
	SSHKeys *string `json:"sshKeys,omitempty"`
	// serviceType determines the Service type that will be created for incoming
	// SSH connections.
	//+optional
	ServiceType *corev1.ServiceType `json:"serviceType,omitempty"`
	// serviceAnnotations defines annotations that will be added to the
	// service created for incoming SSH connections.  If set, these annotations
	// will be used instead of any VolSync default values.
	//+optional
	ServiceAnnotations *map[string]string `json:"serviceAnnotations,omitempty"`
	// address is the remote address to connect to for replication.
	//+optional
	Address *string `json:"address,omitempty"`
	// port is the SSH port to connect to for replication. Defaults to 22.
	//+kubebuilder:validation:Minimum=0
	//+kubebuilder:validation:Maximum=65535
	//+optional
	Port *int32 `json:"port,omitempty"`
	// path is the remote path to rsync from. Defaults to "/"
	//+optional
	Path *string `json:"path,omitempty"`
	// sshUser is the username for outgoing SSH connections. Defaults to "root".
	//+optional
	SSHUser *string `json:"sshUser,omitempty"`
	// MoverServiceAccount allows specifying the name of the service account
	// that will be used by the data mover. This should only be used by advanced
	// users who want to override the service account normally used by the mover.
	// The service account needs to exist in the same namespace as the ReplicationDestination.
	//+optional
	MoverServiceAccount *string `json:"moverServiceAccount,omitempty"`
}

// ReplicationDestinationRcloneSpec defines the field for rclone in replicationDestination.
type ReplicationDestinationRcloneSpec struct {
	ReplicationDestinationVolumeOptions `json:",inline"`
	//RcloneConfigSection is the section in rclone_config file to use for the current job.
	RcloneConfigSection *string `json:"rcloneConfigSection,omitempty"`
	// RcloneDestPath is the remote path to sync to.
	RcloneDestPath *string `json:"rcloneDestPath,omitempty"`
	// RcloneConfig is the rclone secret name
	RcloneConfig *string `json:"rcloneConfig,omitempty"`
	// MoverSecurityContext allows specifying the PodSecurityContext that will
	// be used by the data mover
	MoverSecurityContext *corev1.PodSecurityContext `json:"moverSecurityContext,omitempty"`
	// MoverServiceAccount allows specifying the name of the service account
	// that will be used by the data mover. This should only be used by advanced
	// users who want to override the service account normally used by the mover.
	// The service account needs to exist in the same namespace as the ReplicationDestination.
	//+optional
	MoverServiceAccount *string `json:"moverServiceAccount,omitempty"`
}

// ReplicationDestinationExternalSpec defines the configuration when using an
// external replication provider.
type ReplicationDestinationExternalSpec struct {
	// provider is the name of the external replication provider. The name
	// should be of the form: domain.com/provider.
	Provider string `json:"provider,omitempty"`
	// parameters are provider-specific key/value configuration parameters. For
	// more information, please see the documentation of the specific
	// replication provider being used.
	Parameters map[string]string `json:"parameters,omitempty"`
}

// ReplicationDestinationSpec defines the desired state of
// ReplicationDestination
type ReplicationDestinationSpec struct {
	// trigger determines if/when the destination should attempt to synchronize
	// data with the source.
	//+optional
	Trigger *ReplicationDestinationTriggerSpec `json:"trigger,omitempty"`
	// rsync defines the configuration when using Rsync-based replication.
	//+optional
	Rsync *ReplicationDestinationRsyncSpec `json:"rsync,omitempty"`
	// rsyncTLS defines the configuration when using Rsync-based replication over TLS.
	//+optional
	RsyncTLS *ReplicationDestinationRsyncTLSSpec `json:"rsyncTLS,omitempty"`
	// rclone defines the configuration when using Rclone-based replication.
	//+optional
	Rclone *ReplicationDestinationRcloneSpec `json:"rclone,omitempty"`
	// restic defines the configuration when using Restic-based replication.
	//+optional
	Restic *ReplicationDestinationResticSpec `json:"restic,omitempty"`
	// external defines the configuration when using an external replication
	// provider.
	//+optional
	External *ReplicationDestinationExternalSpec `json:"external,omitempty"`
	// paused can be used to temporarily stop replication. Defaults to "false".
	//+optional
	Paused bool `json:"paused,omitempty"`
}

type ReplicationDestinationRsyncStatus struct {
	// sshKeys is the name of a Secret that contains the SSH keys to be used for
	// authentication. If not provided in .spec.rsync.sshKeys, SSH keys will be
	// generated and the appropriate keys for the remote side will be placed
	// here.
	//+optional
	SSHKeys *string `json:"sshKeys,omitempty"`
	// address is the address to connect to for incoming SSH replication
	// connections.
	//+optional
	Address *string `json:"address,omitempty"`
	// port is the SSH port to connect to for incoming SSH replication
	// connections.
	//+optional
	Port *int32 `json:"port,omitempty"`
}

type ReplicationDestinationResticCA struct {
	// The name of a Secret that contains the custom CA certificate
	SecretName string `json:"secretName,omitempty"`
	// The key within the Secret containing the CA certificate
	Key string `json:"key,omitempty"`
}

// ReplicationDestinationResticSpec defines the field for restic in replicationDestination.
type ReplicationDestinationResticSpec struct {
	ReplicationDestinationVolumeOptions `json:",inline"`
	// Repository is the secret name containing repository info
	Repository string `json:"repository,omitempty"`
	// customCA is a custom CA that will be used to verify the remote
	CustomCA ReplicationDestinationResticCA `json:"customCA,omitempty"`
	// cacheCapacity can be used to set the size of the restic metadata cache volume
	//+optional
	CacheCapacity *resource.Quantity `json:"cacheCapacity,omitempty"`
	// cacheStorageClassName can be used to set the StorageClass of the restic
	// metadata cache volume
	//+optional
	CacheStorageClassName *string `json:"cacheStorageClassName,omitempty"`
	// accessModes can be used to set the accessModes of restic metadata cache volume
	//+optional
	CacheAccessModes []corev1.PersistentVolumeAccessMode `json:"cacheAccessModes,omitempty"`
	// Previous specifies the number of image to skip before selecting one to restore from
	//+optional
	Previous *int32 `json:"previous,omitempty"`
	// RestoreAsOf refers to the backup that is most recent as of that time.
	// +kubebuilder:validation:Format="date-time"
	//+optional
	RestoreAsOf *string `json:"restoreAsOf,omitempty"`
	// MoverSecurityContext allows specifying the PodSecurityContext that will
	// be used by the data mover
	MoverSecurityContext *corev1.PodSecurityContext `json:"moverSecurityContext,omitempty"`
	// MoverServiceAccount allows specifying the name of the service account
	// that will be used by the data mover. This should only be used by advanced
	// users who want to override the service account normally used by the mover.
	// The service account needs to exist in the same namespace as the ReplicationDestination.
	//+optional
	MoverServiceAccount *string `json:"moverServiceAccount,omitempty"`
}

// ReplicationDestinationStatus defines the observed state of ReplicationDestination
type ReplicationDestinationStatus struct {
	// lastSyncTime is the time of the most recent successful synchronization.
	//+optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`
	// lastSyncStartTime is the time the most recent synchronization started.
	//+optional
	LastSyncStartTime *metav1.Time `json:"lastSyncStartTime,omitempty"`
	// lastSyncDuration is the amount of time required to send the most recent
	// update.
	//+optional
	LastSyncDuration *metav1.Duration `json:"lastSyncDuration,omitempty"`
	// nextSyncTime is the time when the next volume synchronization is
	// scheduled to start (for schedule-based synchronization).
	//+optional
	NextSyncTime *metav1.Time `json:"nextSyncTime,omitempty"`
	// lastManualSync is set to the last spec.trigger.manual when the manual sync is done.
	//+optional
	LastManualSync string `json:"lastManualSync,omitempty"`
	// latestImage in the object holding the most recent consistent replicated
	// image.
	//+optional
	LatestImage *corev1.TypedLocalObjectReference `json:"latestImage,omitempty"`
	// Logs/Summary from latest mover job
	//+optional
	LatestMoverStatus *MoverStatus `json:"latestMoverStatus,omitempty"`
	// rsync contains status information for Rsync-based replication.
	Rsync *ReplicationDestinationRsyncStatus `json:"rsync,omitempty"`
	// rsyncTLS contains status information for Rsync-based replication over TLS.
	RsyncTLS *ReplicationDestinationRsyncTLSStatus `json:"rsyncTLS,omitempty"`
	// external contains provider-specific status information. For more details,
	// please see the documentation of the specific replication provider being
	// used.
	//+optional
	External map[string]string `json:"external,omitempty"`
	// conditions represent the latest available observations of the
	// destination's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ReplicationDestination defines the destination for a replicated volume
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Last sync",type="string",format="date-time",JSONPath=`.status.lastSyncTime`
// +kubebuilder:printcolumn:name="Duration",type="string",JSONPath=`.status.lastSyncDuration`
// +kubebuilder:printcolumn:name="Next sync",type="string",format="date-time",JSONPath=`.status.nextSyncTime`
type ReplicationDestination struct {
	metav1.TypeMeta `json:",inline"`
	//+optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// spec is the desired state of the ReplicationDestination, including the
	// replication method to use and its configuration.
	Spec ReplicationDestinationSpec `json:"spec,omitempty"`
	// status is the observed state of the ReplicationDestination as determined
	// by the controller.
	//+optional
	Status *ReplicationDestinationStatus `json:"status,omitempty"`
}

// ReplicationDestinationList contains a list of ReplicationDestination
// +kubebuilder:object:root=true
type ReplicationDestinationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ReplicationDestination `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ReplicationDestination{}, &ReplicationDestinationList{})
}
