/*
Copyright 2022 The VolSync authors.

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

package mover

import "time"

// This file contains a common set of Event "reasons" and "actions" for use by
// the mover implementations when publishing events.

const (
	// SnapshotBindTimeout is the amount of time we should wait before warning
	// that a VolumeSnapshot object is not bound to a VolumeSnapshotContent
	// object.
	SnapshotBindTimeout = 30 * time.Second
	// SnapshotReadyTimeout is the amount of time we should wait before warning
	// that a VolumeSnapshot object is not ready.
	SnapshotReadyTimeout = 120 * time.Second
	// PVCBindTimeout is the time we should wait before warning that a PVC
	// object is not bound to a PV.
	PVCBindTimeout = 120 * time.Second
	// ServiceAddressTimeout is the time we should wait before warning that a
	// Service has not been assigned an address
	ServiceAddressTimeout = 15 * time.Second
)

// Event "reason" strings: Why are we sending an event?
const (
	EvRTransferStarted = "TransferStarted"
	EvRTransferFailed  = "TransferFailed" // Warning
	EvRSnapCreated     = "VolumeSnapshotCreated"
	EvRSnapNotBound    = "VolumeSnapshotNotBound" // Warning
	EvRSnapNotReady    = "VolumeSnapshotNotReady" // Warning
	EvRPVCCreated      = "PersistentVolumeClaimCreated"
	EvRPVCNotBound     = "PersistentVolumeClaimNotBound" // Warning
	EvRSvcAddress      = "ServiceAddressAssigned"
	EvRSvcNoAddress    = "NoServiceAddressAssigned" // Warning
)

// Event "action" strings: Things the controller "does"
const (
	EvANone        = "" // No action
	EvACreateMover = "CreateMover"
	EvADeleteMover = "DeleteMover"
	EvACreatePVC   = "CreatePersistentVolumeClaim"
	EvACreateSnap  = "CreateVolumeSnapshot"
)
