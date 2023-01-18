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
)

const (
	ConditionSynchronizing     string = "Synchronizing"
	SynchronizingReasonSync    string = "SyncInProgress"
	SynchronizingReasonSched   string = "WaitingForSchedule"
	SynchronizingReasonManual  string = "WaitingForManual"
	SynchronizingReasonCleanup string = "CleaningUp"
	SynchronizingReasonError   string = "Error"
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
