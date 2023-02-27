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

// ReplicationSourceTriggerSpec defines when a volume will be synchronized with
// the destination.
type ReplicationSourceTriggerSpec struct {
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

// ReplicationSourceExternalSpec defines the configuration when using an
// external replication provider.
type ReplicationSourceExternalSpec struct {
	// provider is the name of the external replication provider. The name
	// should be of the form: domain.com/provider.
	Provider string `json:"provider,omitempty"`
	// parameters are provider-specific key/value configuration parameters. For
	// more information, please see the documentation of the specific
	// replication provider being used.
	Parameters map[string]string `json:"parameters,omitempty"`
}

type ReplicationSourceVolumeOptions struct {
	// copyMethod describes how a point-in-time (PiT) image of the source volume
	// should be created.
	CopyMethod CopyMethodType `json:"copyMethod,omitempty"`
	// capacity can be used to override the capacity of the PiT image.
	//+optional
	Capacity *resource.Quantity `json:"capacity,omitempty"`
	// storageClassName can be used to override the StorageClass of the PiT
	// image.
	//+optional
	StorageClassName *string `json:"storageClassName,omitempty"`
	// accessModes can be used to override the accessModes of the PiT image.
	//+kubebuilder:validation:MinItems=1
	//+optional
	AccessModes []corev1.PersistentVolumeAccessMode `json:"accessModes,omitempty"`
	// volumeSnapshotClassName can be used to specify the VSC to be used if
	// copyMethod is Snapshot. If not set, the default VSC is used.
	//+optional
	VolumeSnapshotClassName *string `json:"volumeSnapshotClassName,omitempty"`
}

type ReplicationSourceRsyncSpec struct {
	ReplicationSourceVolumeOptions `json:",inline"`
	// sshKeys is the name of a Secret that contains the SSH keys to be used for
	// authentication. If not provided, the keys will be generated.
	//+optional
	SSHKeys *string `json:"sshKeys,omitempty"`
	// serviceType determines the Service type that will be created for incoming
	// SSH connections.
	//+optional
	ServiceType *corev1.ServiceType `json:"serviceType,omitempty"`
	// address is the remote address to connect to for replication.
	//+optional
	Address *string `json:"address,omitempty"`
	// port is the SSH port to connect to for replication. Defaults to 22.
	//+kubebuilder:validation:Minimum=0
	//+kubebuilder:validation:Maximum=65535
	//+optional
	Port *int32 `json:"port,omitempty"`
	// path is the remote path to rsync to. Defaults to "/"
	//+optional
	Path *string `json:"path,omitempty"`
	// sshUser is the username for outgoing SSH connections. Defaults to "root".
	//+optional
	SSHUser *string `json:"sshUser,omitempty"`
	// MoverServiceAccount allows specifying the name of the service account
	// that will be used by the data mover. This should only be used by advanced
	// users who want to override the service account normally used by the mover.
	// The service account needs to exist in the same namespace as the ReplicationSource.
	//+optional
	MoverServiceAccount *string `json:"moverServiceAccount,omitempty"`
}

// ReplicationSourceRcloneSpec defines the field for rclone in replicationSource.
type ReplicationSourceRcloneSpec struct {
	ReplicationSourceVolumeOptions `json:",inline"`
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
	// The service account needs to exist in the same namespace as the ReplicationSource.
	//+optional
	MoverServiceAccount *string `json:"moverServiceAccount,omitempty"`
}

// ResticRetainPolicy defines the feilds for Restic backup
type ResticRetainPolicy struct {
	// Hourly defines the number of snapshots to be kept hourly
	//+optional
	Hourly *int32 `json:"hourly,omitempty"`
	// Daily defines the number of snapshots to be kept daily
	//+optional
	Daily *int32 `json:"daily,omitempty"`
	// Weekly defines the number of snapshots to be kept weekly
	//+optional
	Weekly *int32 `json:"weekly,omitempty"`
	// Monthly defines the number of snapshots to be kept monthly
	//+optional
	Monthly *int32 `json:"monthly,omitempty"`
	// Yearly defines the number of snapshots to be kept yearly
	//+optional
	Yearly *int32 `json:"yearly,omitempty"`
	// Within defines the number of snapshots to be kept Within the given time period
	//+optional
	Within *string `json:"within,omitempty"`
}

type ReplicationSourceResticCA struct {
	// The name of a Secret that contains the custom CA certificate
	SecretName string `json:"secretName,omitempty"`
	// The key within the Secret containing the CA certificate
	Key string `json:"key,omitempty"`
}

// ReplicationSourceResticSpec defines the field for restic in replicationSource.
type ReplicationSourceResticSpec struct {
	ReplicationSourceVolumeOptions `json:",inline"`
	// PruneIntervalDays define how often to prune the repository
	PruneIntervalDays *int32 `json:"pruneIntervalDays,omitempty"`
	// Repository is the secret name containing repository info
	Repository string `json:"repository,omitempty"`
	// customCA is a custom CA that will be used to verify the remote
	CustomCA ReplicationSourceResticCA `json:"customCA,omitempty"`
	// ResticRetainPolicy define the retain policy
	//+optional
	Retain *ResticRetainPolicy `json:"retain,omitempty"`
	// cacheCapacity can be used to set the size of the restic metadata cache volume
	//+optional
	CacheCapacity *resource.Quantity `json:"cacheCapacity,omitempty"`
	// cacheStorageClassName can be used to set the StorageClass of the restic
	// metadata cache volume
	//+optional
	CacheStorageClassName *string `json:"cacheStorageClassName,omitempty"`
	// CacheAccessModes can be used to set the accessModes of restic metadata cache volume
	//+optional
	CacheAccessModes []corev1.PersistentVolumeAccessMode `json:"cacheAccessModes,omitempty"`
	// MoverSecurityContext allows specifying the PodSecurityContext that will
	// be used by the data mover
	MoverSecurityContext *corev1.PodSecurityContext `json:"moverSecurityContext,omitempty"`
	// MoverServiceAccount allows specifying the name of the service account
	// that will be used by the data mover. This should only be used by advanced
	// users who want to override the service account normally used by the mover.
	// The service account needs to exist in the same namespace as the ReplicationSource.
	//+optional
	MoverServiceAccount *string `json:"moverServiceAccount,omitempty"`
}

// ReplicationSourceResticStatus defines the field for ReplicationSourceStatus in ReplicationSourceStatus
type ReplicationSourceResticStatus struct {
	// lastPruned in the object holding the time of last pruned
	//+optional
	LastPruned *metav1.Time `json:"lastPruned,omitempty"`
}

// define the Syncthing field
type ReplicationSourceSyncthingSpec struct {
	// List of Syncthing peers to be connected for syncing
	Peers []SyncthingPeer `json:"peers,omitempty"`
	// Type of service to be used when exposing the Syncthing peer
	//+optional
	ServiceType *corev1.ServiceType `json:"serviceType,omitempty"`
	// Used to set the size of the Syncthing config volume.
	//+optional
	ConfigCapacity *resource.Quantity `json:"configCapacity,omitempty"`
	// Used to set the StorageClass of the Syncthing config volume.
	//+optional
	ConfigStorageClassName *string `json:"configStorageClassName,omitempty"`
	// Used to set the accessModes of Syncthing config volume.
	//+optional
	ConfigAccessModes []corev1.PersistentVolumeAccessMode `json:"configAccessModes,omitempty"`
	// MoverSecurityContext allows specifying the PodSecurityContext that will
	// be used by the data mover
	MoverSecurityContext *corev1.PodSecurityContext `json:"moverSecurityContext,omitempty"`
	// MoverServiceAccount allows specifying the name of the service account
	// that will be used by the data mover. This should only be used by advanced
	// users who want to override the service account normally used by the mover.
	// The service account needs to exist in the same namespace as the ReplicationSource.
	//+optional
	MoverServiceAccount *string `json:"moverServiceAccount,omitempty"`
}

// ReplicationSourceSpec defines the desired state of ReplicationSource
type ReplicationSourceSpec struct {
	// sourcePVC is the name of the PersistentVolumeClaim (PVC) to replicate.
	SourcePVC string `json:"sourcePVC,omitempty"`
	// trigger determines when the latest state of the volume will be captured
	// (and potentially replicated to the destination).
	//+optional
	Trigger *ReplicationSourceTriggerSpec `json:"trigger,omitempty"`
	// rsync defines the configuration when using Rsync-based replication.
	//+optional
	Rsync *ReplicationSourceRsyncSpec `json:"rsync,omitempty"`
	// rsyncTLS defines the configuration when using Rsync-based replication over TLS.
	//+optional
	RsyncTLS *ReplicationSourceRsyncTLSSpec `json:"rsyncTLS,omitempty"`
	// rclone defines the configuration when using Rclone-based replication.
	//+optional
	Rclone *ReplicationSourceRcloneSpec `json:"rclone,omitempty"`
	// restic defines the configuration when using Restic-based replication.
	//+optional
	Restic *ReplicationSourceResticSpec `json:"restic,omitempty"`
	// syncthing defines the configuration when using Syncthing-based replication.
	//+optional
	Syncthing *ReplicationSourceSyncthingSpec `json:"syncthing,omitempty"`
	// external defines the configuration when using an external replication
	// provider.
	//+optional
	External *ReplicationSourceExternalSpec `json:"external,omitempty"`
	// paused can be used to temporarily stop replication. Defaults to "false".
	//+optional
	Paused bool `json:"paused,omitempty"`
}

type ReplicationSourceRsyncStatus struct {
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

type ReplicationSourceSyncthingStatus struct {
	// List of the Syncthing nodes we are currently connected to.
	Peers []SyncthingPeerStatus `json:"peers,omitempty"`
	// Device ID of the current syncthing device
	ID string `json:"ID,omitempty"`
	// Service address where Syncthing is exposed to the rest of the world
	Address string `json:"address,omitempty"`
}

// ReplicationSourceStatus defines the observed state of ReplicationSource
type ReplicationSourceStatus struct {
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
	// Logs/Summary from latest mover job
	//+optional
	LatestMoverStatus *MoverStatus `json:"latestMoverStatus,omitempty"`
	// rsync contains status information for Rsync-based replication.
	Rsync *ReplicationSourceRsyncStatus `json:"rsync,omitempty"`
	// rsyncTLS contains status information for Rsync-based replication over TLS.
	RsyncTLS *ReplicationSourceRsyncTLSStatus `json:"rsyncTLS,omitempty"`
	// external contains provider-specific status information. For more details,
	// please see the documentation of the specific replication provider being
	// used.
	//+optional
	External map[string]string `json:"external,omitempty"`
	// conditions represent the latest available observations of the
	// source's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// restic contains status information for Restic-based replication.
	Restic *ReplicationSourceResticStatus `json:"restic,omitempty"`
	// contains status information when Syncthing-based replication is used.
	//+optional
	Syncthing *ReplicationSourceSyncthingStatus `json:"syncthing,omitempty"`
}

// ReplicationSource defines the source for a replicated volume
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Source",type="string",JSONPath=`.spec.sourcePVC`
// +kubebuilder:printcolumn:name="Last sync",type="string",format="date-time",JSONPath=`.status.lastSyncTime`
// +kubebuilder:printcolumn:name="Duration",type="string",JSONPath=`.status.lastSyncDuration`
// +kubebuilder:printcolumn:name="Next sync",type="string",format="date-time",JSONPath=`.status.nextSyncTime`
type ReplicationSource struct {
	metav1.TypeMeta `json:",inline"`
	//+optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// spec is the desired state of the ReplicationSource, including the
	// replication method to use and its configuration.
	Spec ReplicationSourceSpec `json:"spec,omitempty"`
	// status is the observed state of the ReplicationSource as determined by
	// the controller.
	//+optional
	Status *ReplicationSourceStatus `json:"status,omitempty"`
}

// ReplicationSourceList contains a list of Source
// +kubebuilder:object:root=true
type ReplicationSourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ReplicationSource `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ReplicationSource{}, &ReplicationSourceList{})
}
