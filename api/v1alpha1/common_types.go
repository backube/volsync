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

package v1alpha1

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// CopyMethodType defines the methods for creating point-in-time copies of
// volumes.
// +kubebuilder:validation:Enum=Direct;None;Clone;Snapshot
type CopyMethodType string

const (
	// CopyMethodDirect indicates a copy should not be performed. Data will be copied directly to/from the PVC.
	CopyMethodDirect CopyMethodType = "Direct"
	// CopyMethodNone indicates a copy should not be performed. Deprecated (replaced by CopyMethodDirect).
	CopyMethodNone CopyMethodType = "None"
	// CopyMethodClone indicates a copy should be created using volume cloning.
	CopyMethodClone CopyMethodType = "Clone"
	// CopyMethodSnapshot indicates a copy should be created using a volume
	// snapshot.
	CopyMethodSnapshot CopyMethodType = "Snapshot"

	// Namespace annotation to indicate that elevated permissions are ok for movers
	PrivilegedMoversNamespaceAnnotation = "volsync.backube/privileged-movers"

	// Annotation on ReplicationSource or ReplicationDestination to enable running the mover job in debug mode
	EnableDebugMoverAnnotation = "volsync.backube/enable-debug-mover"
)

const (
	ConditionSynchronizing    string = "Synchronizing"
	SynchronizingReasonSync   string = "SyncInProgress"
	SynchronizingReasonSched  string = "WaitingForSchedule"
	SynchronizingReasonManual string = "WaitingForManual"
	SynchronizingReasonError  string = "Error"
)

const (
	// Annotation optionally set on src pvc by user.  When set, a volsync source replication
	// that is using CopyMode: Snapshot or Clone will wait for the user to set a unique copy-trigger
	// before proceeding to take the src snapshot/clone.
	UseCopyTriggerAnnotation = "volsync.backube/use-copy-trigger"
	CopyTriggerAnnotation    = "volsync.backube/copy-trigger"

	// Annotations for status set by VolSync on a src pvc if UseCopyTriggerAnnotation is set to "true"
	LatestCopyTriggerAnnotation             = "volsync.backube/latest-copy-trigger"
	LatestCopyStatusAnnotation              = "volsync.backube/latest-copy-status"
	LatestCopyTriggerWaitingSinceAnnotation = "volsync.backube/latest-copy-trigger-waiting-since"

	// VolSync latest-copy-status annotation values
	LatestCopyStatusValueWaitingForTrigger = "WaitingForTrigger"
	LatestCopyStatusValueInProgress        = "InProgress"
	LatestCopyStatusValueCompleted         = "Completed"

	// Timeout before we start updating the latestMoverStatus with an error
	// (After timeout we still continue to sync and wait for the copy-trigger to be
	//  set/updated)
	CopyTriggerWaitTimeout time.Duration = 10 * time.Minute
)

// SyncthingPeer Defines the necessary information needed by VolSync
// to configure a given peer with the running Syncthing instance.
type SyncthingPeer struct {
	// The peer's address that our Syncthing node will connect to.
	Address string `json:"address"`
	// The peer's Syncthing ID.
	ID string `json:"ID"`
	// A flag that determines whether this peer should
	// introduce us to other peers sharing this volume.
	// It is HIGHLY recommended that two Syncthing peers do NOT
	// set each other as introducers as you will have a difficult time
	// disconnecting the two.
	Introducer bool `json:"introducer"`
}

// SyncthingPeerStatus Is a struct that contains information pertaining to
// the status of a given Syncthing peer.
type SyncthingPeerStatus struct {
	// The address of the Syncthing peer.
	Address string `json:"address"`
	// ID Is the peer's Syncthing ID.
	ID string `json:"ID"`
	// Flag indicating whether peer is currently connected.
	Connected bool `json:"connected"`
	// The ID of the Syncthing peer that this one was introduced by.
	IntroducedBy string `json:"introducedBy,omitempty"`
	// A friendly name to associate the given device.
	Name string `json:"name,omitempty"`
}

type MoverResult string

const (
	MoverResultSuccessful MoverResult = "Successful"
	MoverResultFailed     MoverResult = "Failed"
)

type MoverStatus struct {
	Result MoverResult `json:"result,omitempty"`
	Logs   string      `json:"logs,omitempty"`
}

type CustomCASpec struct {
	// The name of a Secret that contains the custom CA certificate
	// If SecretName is used then ConfigMapName should not be set
	SecretName string `json:"secretName,omitempty"`

	// The name of a ConfigMap that contains the custom CA certificate
	// If ConfigMapName is used then SecretName should not be set
	ConfigMapName string `json:"configMapName,omitempty"`

	// The key within the Secret or ConfigMap containing the CA certificate
	Key string `json:"key,omitempty"`
}

// KopiaPolicySpec defines configuration for Kopia policy files
type KopiaPolicySpec struct {
	// The name of a Secret that contains Kopia policy configuration files
	// If SecretName is used then ConfigMapName should not be set
	SecretName string `json:"secretName,omitempty"`

	// The name of a ConfigMap that contains Kopia policy configuration files
	// If ConfigMapName is used then SecretName should not be set
	ConfigMapName string `json:"configMapName,omitempty"`

	// GlobalPolicyFilename specifies the filename for the global policy configuration.
	// This file should contain a JSON policy configuration that will be applied globally.
	// Defaults to "global-policy.json" if not specified.
	//+optional
	GlobalPolicyFilename string `json:"globalPolicyFilename,omitempty"`

	// RepositoryConfigFilename specifies the filename for the repository configuration.
	// This file should contain repository-specific settings like actions enablement.
	// Defaults to "repository.config" if not specified.
	//+optional
	RepositoryConfigFilename string `json:"repositoryConfigFilename,omitempty"`

	// RepositoryConfig is a multiline JSON string containing Kopia repository configuration
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Optional
	RepositoryConfig *string `json:"repositoryConfig,omitempty"`
}

type MoverConfig struct {
	// MoverSecurityContext allows specifying the PodSecurityContext that will
	// be used by the data mover
	MoverSecurityContext *corev1.PodSecurityContext `json:"moverSecurityContext,omitempty"`
	// MoverServiceAccount allows specifying the name of the service account
	// that will be used by the data mover. This should only be used by advanced
	// users who want to override the service account normally used by the mover.
	// The service account needs to exist in the same namespace as this CR.
	//+optional
	MoverServiceAccount *string `json:"moverServiceAccount,omitempty"`
	// Labels that should be added to data mover pods
	// These will be in addition to any labels that VolSync may add
	// +optional
	MoverPodLabels map[string]string `json:"moverPodLabels,omitempty"`
	// Resources represents compute resources required by the data mover container.
	// Immutable.
	// More info: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/
	// This should only be used by advanced users as this can result in a mover
	// pod being unschedulable or crashing due to limited resources.
	// +optional
	MoverResources *corev1.ResourceRequirements `json:"moverResources,omitempty"`
	// MoverAffinity allows specifying the PodAffinity that will be used by the data mover
	MoverAffinity *corev1.Affinity `json:"moverAffinity,omitempty"`
}
