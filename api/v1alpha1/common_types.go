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
	"github.com/operator-framework/operator-lib/status"
)

// CopyMethodType defines the methods for creating point-in-time copies of
// volumes.
//+kubebuilder:validation:Enum=None;Clone;Snapshot
type CopyMethodType string

const (
	// CopyMethodNone indicates a copy should not be performed.
	CopyMethodNone CopyMethodType = "None"
	// CopyMethodClone indicates a copy should be created using volume cloning.
	CopyMethodClone CopyMethodType = "Clone"
	// CopyMethodSnapshot indicates a copy should be created using a volume
	// snapshot.
	CopyMethodSnapshot CopyMethodType = "Snapshot"
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

const (
	ConditionSynchronizing     status.ConditionType   = "Synchronizing"
	SynchronizingReasonSync    status.ConditionReason = "SyncInProgress"
	SynchronizingReasonSched   status.ConditionReason = "WaitingForSchedule"
	SynchronizingReasonManual  status.ConditionReason = "WaitingForManual"
	SynchronizingReasonCleanup status.ConditionReason = "CleaningUp"
)
