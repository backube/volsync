/*
Copyright 2023 The VolSync authors.

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

package v1alpha1

// This file contains a common set of Event "reasons" and "actions" for use by
// the mover implementations when publishing events.

// ReplicationSource/ReplicationDestination Event "reason" strings: Why are we sending an event?
const (
	EvRTransferStarted = "TransferStarted"
	EvRTransferFailed  = "TransferFailed" // Warning
	EvRSnapCreated     = "VolumeSnapshotCreated"
	EvRSnapNotBound    = "VolumeSnapshotNotBound" // Warning
	EvRPVCCreated      = "PersistentVolumeClaimCreated"
	EvRPVCNotBound     = "PersistentVolumeClaimNotBound" // Warning
	EvRSvcAddress      = "ServiceAddressAssigned"
	EvRSvcNoAddress    = "NoServiceAddressAssigned" // Warning
)

// ReplicationSource/ReplicationDestination Event "action" strings: Things the controller "does"
const (
	EvANone        = "" // No action
	EvACreateMover = "CreateMover"
	EvADeleteMover = "DeleteMover"
	EvACreatePVC   = "CreatePersistentVolumeClaim"
	EvACreateSnap  = "CreateVolumeSnapshot"
)

// Volume Populator Event "reason" strings
const (
	EvRVolPopPVCPopulatorFinished            = "VolSyncPopulatorFinished" // #nosec G101 - gosec thinks this is a cred
	EvRVolPopPVCPopulatorError               = "VolSyncPopulatorError"
	EvVolPopPVCReplicationDestMissing        = "VolSyncPopulatorReplicationDestinationMissing"
	EvRVolPopPVCReplicationDestNoLatestImage = "VolSyncPopulatorReplicationDestinationNoLatestImage"
	EvRVolPopPVCCreationSuccess              = "VolSyncPopulatorPVCCreated"
	EvRVolPopPVCCreationError                = "VolSyncPopulatorPVCCreationError"
)
