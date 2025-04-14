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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/backube/volsync/internal/controller/mover"
)

// fakeMachine is a mock ReplicationMachine used for testing
type fakeMachine struct {
	TT                  triggerType
	CS                  string
	MT                  string
	LMT                 string
	NST                 *metav1.Time
	LSST                *metav1.Time
	LST                 *metav1.Time
	LSD                 *metav1.Duration
	Cond                []metav1.Condition
	OOSync              bool
	MissedIntervals     int
	DurationObservation time.Duration
	SyncResult          mover.Result
	SyncErr             error
	CleanupResult       mover.Result
	CleanupError        error
}

var _ ReplicationMachine = &fakeMachine{}

func newFakeMachine() *fakeMachine {
	return &fakeMachine{
		TT:            noTrigger,
		SyncResult:    mover.Complete(),
		CleanupResult: mover.Complete(),
	}
}

func (f *fakeMachine) Cronspec() string                       { return f.CS }
func (f *fakeMachine) ManualTag() string                      { return f.MT }
func (f *fakeMachine) LastManualTag() string                  { return f.LMT }
func (f *fakeMachine) SetLastManualTag(t string)              { f.LMT = t }
func (f *fakeMachine) NextSyncTime() *metav1.Time             { return f.NST }
func (f *fakeMachine) SetNextSyncTime(t *metav1.Time)         { f.NST = t }
func (f *fakeMachine) LastSyncStartTime() *metav1.Time        { return f.LSST }
func (f *fakeMachine) SetLastSyncStartTime(t *metav1.Time)    { f.LSST = t }
func (f *fakeMachine) LastSyncTime() *metav1.Time             { return f.LST }
func (f *fakeMachine) SetLastSyncTime(t *metav1.Time)         { f.LST = t }
func (f *fakeMachine) LastSyncDuration() *metav1.Duration     { return f.LSD }
func (f *fakeMachine) SetLastSyncDuration(d *metav1.Duration) { f.LSD = d }
func (f *fakeMachine) Conditions() *[]metav1.Condition        { return &f.Cond }
func (f *fakeMachine) SetOutOfSync(oos bool)                  { f.OOSync = oos }
func (f *fakeMachine) IncMissedIntervals()                    { f.MissedIntervals++ }
func (f *fakeMachine) ObserveSyncDuration(t time.Duration)    { f.DurationObservation = t }
func (f *fakeMachine) Synchronize(_ context.Context) (mover.Result, error) {
	return f.SyncResult, f.SyncErr
}
func (f *fakeMachine) Cleanup(_ context.Context) (mover.Result, error) {
	return f.CleanupResult, f.CleanupError
}
