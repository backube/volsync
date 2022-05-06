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

package statemachine

import (
	"context"
	"time"

	"github.com/backube/volsync/controllers/mover"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ReplicationMachine is a common interface to the ReplicationSource and
// ReplicationDestination types that allow us to generically implement the
// synchronization state machine.
type ReplicationMachine interface {
	Cronspec() string
	ManualTag() string
	LastManualTag() string
	SetLastManualTag(string)

	NextSyncTime() *metav1.Time
	SetNextSyncTime(*metav1.Time)

	LastSyncStartTime() *metav1.Time
	SetLastSyncStartTime(*metav1.Time)

	LastSyncTime() *metav1.Time
	SetLastSyncTime(*metav1.Time)

	LastSyncDuration() *metav1.Duration
	SetLastSyncDuration(*metav1.Duration)

	Conditions() *[]metav1.Condition

	SetOutOfSync(bool)
	IncMissedIntervals()
	ObserveSyncDuration(time.Duration)

	Synchronize(ctx context.Context) (mover.Result, error)
	Cleanup(ctx context.Context) (mover.Result, error)
}
