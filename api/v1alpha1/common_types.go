/*
Copyright 2020 The Scribe authors.

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
	"github.com/operator-framework/operator-lib/status"
)

// ReplicationMethodType describes values that can be used to specify the
// replication method in the ReplicationSource and ReplicationDestination CRs
type ReplicationMethodType string

const (
	// ReplicationMethodRsync is the replication method that uses the built-in
	// rsync over ssh data mover to replicate data
	ReplicationMethodRsync ReplicationMethodType = "Rsync"
)

// Parameters used by the Rsync replication method in *.Spec.Parameters and
// *.Status.MethodStatus
const (
	// RsyncServiceTypeKey determines the ServiceType that is created
	// to handle the incoming connection to the destination. It should be one of
	// the values supported by Service.spec.type
	RsyncServiceTypeKey = "serviceType"
	// RsyncStorageClassNameKey determines the StorageClass name used
	// to create temporary PVCs for replication. This should match the Source
	// PVC on the sending side and the Destination PVC on the receiving side.
	RsyncStorageClassNameKey = "storageClassName"
	// RsyncVolumeSnapshotClassNameKey is the name of the volume
	// snapshot class that should be used to create PVC snapshots. It must be a
	// class that is compatible with the StorageClass being used.
	RsyncVolumeSnapshotClassNameKey = "volumeSnapshotClassName"
	// RsyncAccessModeKey is the access mode (ReadWriteOnce or
	// ReadWriteMany) of the temporary PVCs. This should match the access mode
	// of the source/destination volumes.
	RsyncAccessModeKey = "accessMode"
	// RsyncCapacityKey is the capacity to use when creating the
	// temporary volumes. It should match the size of the source/destination
	// volumes.
	RsyncCapacityKey = "capacity"
	//RsyncConsistencyModeKey = "consistencyMode"
	RsyncConnectionInfoKey = "connectionInfo"
	// RsyncLatestSnapKey is the name of the snapshot holding the most recently
	// synchronized data
	RsyncLatestSnapKey = "latestSnapshot"
)

const (
	// ConditionReconciled is a status condition type that indicates whether the
	// CR has been successfully reconciled
	ConditionReconciled status.ConditionType = "Reconciled"
	// ReconciledReasonComplete indicates the CR was successfully reconciled
	ReconciledReasonComplete status.ConditionReason = "ReconcileComplete"
	// ReconciledReasonError indicates an error was encountered while
	// reconciling the CR
	ReconciledReasonError status.ConditionReason = "ReconcileError"
)
