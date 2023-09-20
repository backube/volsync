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

	"github.com/go-logr/logr"
	cron "github.com/robfig/cron/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

// replicationState is the different states that replication object can be in
type replicationState string

const (
	initialState       replicationState = "Initial"
	synchronizingState replicationState = "Synchronizing"
	cleaningUpState    replicationState = "CleaningUp"
)

// triggerType represents the different ways we can trigger data synchronization
type triggerType string

const (
	scheduleTrigger triggerType = "ScheduleTrigger"
	manualTrigger   triggerType = "ManualTrigger"
	noTrigger       triggerType = "NoTrigger"
)

// Run the state machine to reconcile the ReplicationController
func Run(ctx context.Context, r ReplicationMachine, l logr.Logger) (ctrl.Result, error) {
	// Set out-of-sync metrics flag if necessary
	if r.LastSyncTime() == nil {
		r.SetOutOfSync(true)
	} else {
		missed, err := missedDeadline(r)
		if err != nil {
			setConditionError(r, l, err)
			return ctrl.Result{}, err
		}
		if missed {
			r.SetOutOfSync(true)
		}
	}

	var result ctrl.Result
	var err error
	switch currentState(r) {
	case initialState:
		result, err = doInitialState(ctx, r, l)
	case synchronizingState:
		result, err = doSynchronizingState(ctx, r, l)
	case cleaningUpState:
		result, err = doCleanupState(ctx, r, l)
	default:
		l.Error(nil, "invalid state detected; switching to Synchronizing")
		err = transitionToSynchronizing(r, l)
	}
	if err != nil {
		setConditionError(r, l, err)
	}
	return result, err
}

func getTrigger(r ReplicationMachine) triggerType {
	switch {
	case len(r.ManualTag()) > 0:
		return manualTrigger
	case len(r.Cronspec()) > 0:
		return scheduleTrigger
	default:
		return noTrigger
	}
}

// ctrl.Result is always empty, but leave it as a return param to be consistent with other funcs
// nolint:unparam
func doInitialState(_ context.Context, r ReplicationMachine, l logr.Logger) (ctrl.Result, error) {
	err := transitionToSynchronizing(r, l)
	// We don't need to explicitly re-queue because the transition will
	// cause a .status update
	return ctrl.Result{}, err
}

func doSynchronizingState(ctx context.Context, r ReplicationMachine, l logr.Logger) (ctrl.Result, error) {
	result, err := r.Synchronize(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	if result.Completed {
		// Just finished a sync, so we're in-sync
		r.SetOutOfSync(false)
		err = transitionToCleaningUp(r, l)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else {
		setConditionSyncing(r, l)
	}
	return result.ReconcileResult(), nil
}

func doCleanupState(ctx context.Context, r ReplicationMachine, l logr.Logger) (ctrl.Result, error) {
	result, err := r.Cleanup(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Ensure nextSyncTime picks up any changes made to the schedule
	if err := updateNextSyncStartTime(r, l); err != nil {
		return ctrl.Result{}, err
	}

	// If we have finished cleaning up, we remain in this state until the
	// next reconcile is triggered, but we tell the user that we are "idle".
	if result.Completed {
		if shouldSync(r, l) { // Time to start syncing again
			err := transitionToSynchronizing(r, l)
			if err != nil {
				return ctrl.Result{}, err
			}
		} else { // We're idle
			if getTrigger(r) == scheduleTrigger {
				setConditionScheduled(r, l)
			} else {
				setConditionManual(r, l)
			}

			timeToNext := timeToNextSync(r)
			switch {
			case timeToNext == nil:
				return ctrl.Result{}, nil
			default:
				return ctrl.Result{RequeueAfter: *timeToNext}, nil
			}
		}
	} else {
		setConditionCleanup(r, l)
	}
	return result.ReconcileResult(), nil
}

// Determine which state we're in by looking at the CR
func currentState(r ReplicationMachine) replicationState {
	// If we've never completed a sync and we're not trying to sync, we must be
	// in the initial state
	if r.LastSyncTime().IsZero() && r.LastSyncStartTime().IsZero() {
		return initialState
	}
	// If we're trying to sync, then we're in the synchronizing state
	if !r.LastSyncStartTime().IsZero() {
		return synchronizingState
	}
	// Otherwise, we're in cleanup
	return cleaningUpState
}

//nolint:unparam
func transitionToSynchronizing(r ReplicationMachine, l logr.Logger) error {
	l.V(1).Info("transitioning to synchronization state")
	now := metav1.Now()
	r.SetLastSyncStartTime(&now)
	setConditionSyncing(r, l)
	return nil
}

func transitionToCleaningUp(r ReplicationMachine, l logr.Logger) error {
	l.V(1).Info("transitioning to cleanup state")

	// If we took too long, update the miss count. We update here since
	// we only want to count each miss once (ideally), so we do the
	// update only when we try to transition.
	missed, err := missedDeadline(r)
	if err != nil {
		return err
	}
	if missed {
		r.IncMissedIntervals()
	}

	// Record the synchronization end time
	now := metav1.Now()
	r.SetLastSyncTime(&now)

	// Calculate how long the synchronization took
	syncDuration := now.Sub(r.LastSyncStartTime().Time)
	r.SetLastSyncDuration(&metav1.Duration{Duration: syncDuration})
	r.ObserveSyncDuration(syncDuration)

	// Determine when our next synchronization should start
	if err := updateNextSyncStartTime(r, l); err != nil {
		return err
	}

	// Update manual trigger tag in .status to match the one in .spec
	r.SetLastManualTag(r.ManualTag())

	// Since we're done syncing, clear LSST. In addition to being useful for
	// duration calculation, it serves as the indicator of which state we're in
	r.SetLastSyncStartTime(nil)

	setConditionCleanup(r, l)
	return nil
}

// Given that we've finished cleanup, should we start syncing again?
func shouldSync(r ReplicationMachine, l logr.Logger) bool {
	switch getTrigger(r) {
	case scheduleTrigger:
		// When schedule-based, we trigger a sync once we pass the appointed
		// time
		return time.Now().After(r.NextSyncTime().Time)
	case manualTrigger:
		// We need to do a sync if the manual trigger tags don't match
		return r.ManualTag() != r.LastManualTag()
	case noTrigger:
		// When there's no trigger specified, we run in a tight loop,
		// immediately synchronizing as soon as we finish cleanup
		return true
	}
	// We should never get here
	l.Error(nil, "unable to determine whether to sync; defaulting to true")
	return true
}

// How long long until the next sync should start (or nil if not
// schedule-based).
func timeToNextSync(r ReplicationMachine) *time.Duration {
	if r.NextSyncTime().IsZero() {
		return nil
	}
	next := time.Until(r.NextSyncTime().Time)
	return &next
}

func getSchedule(cronspec string) (cron.Schedule, error) {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	return parser.Parse(cronspec)
}

// pastScheduleDeadline returns true if a scheduled sync hasn't been completed
// within the synchronization period.
func pastScheduleDeadline(schedule cron.Schedule, lastCompleted time.Time, now time.Time) bool {
	// Each synchronization should complete before the next scheduled start
	// time. This means that, starting from the last completed, the next sync
	// would start at last->next, and must finish before last->next->next.
	return schedule.Next(schedule.Next(lastCompleted)).Before(now)
}

// Returns true if we're schedule-based and have missed our deadline
func missedDeadline(r ReplicationMachine) (bool, error) {
	if getTrigger(r) == scheduleTrigger && !r.LastSyncTime().IsZero() {
		schedule, err := getSchedule(r.Cronspec())
		if err != nil {
			return false, err
		}
		if pastScheduleDeadline(schedule, r.LastSyncTime().Time, time.Now()) {
			return true, nil
		}
	}
	return false, nil
}

func updateNextSyncStartTime(r ReplicationMachine, l logr.Logger) error {
	lastSync := r.LastSyncTime()

	switch getTrigger(r) {
	case scheduleTrigger:
		schedule, err := getSchedule(r.Cronspec())
		if err != nil {
			l.Error(err, "error parsing schedule", "cronspec", r.Cronspec())
			return err
		}
		next := schedule.Next(lastSync.Time)
		r.SetNextSyncTime(&metav1.Time{Time: next})
	case manualTrigger, noTrigger:
		r.SetNextSyncTime(nil)
	}

	return nil
}
