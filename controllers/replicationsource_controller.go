/*
Copyright 2020 The VolSync authors.

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

package controllers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1beta1"
	"github.com/prometheus/client_golang/prometheus"
	cron "github.com/robfig/cron/v3"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/controllers/mover"
)

// ReplicationSourceReconciler reconciles a ReplicationSource object
type ReplicationSourceReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

//nolint:lll
//nolint:funlen
//+kubebuilder:rbac:groups=volsync.backube,resources=replicationsources,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=volsync.backube,resources=replicationsources/finalizers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=volsync.backube,resources=replicationsources/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete;deletecollection
//+kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete;deletecollection
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,resourceNames=volsync-mover,verbs=use
//+kubebuilder:rbac:groups=snapshot.storage.k8s.io,resources=volumesnapshots,verbs=get;list;watch;create;update;patch;delete;deletecollection

//nolint:funlen
func (r *ReplicationSourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("replicationsource", req.NamespacedName)
	inst := &volsyncv1alpha1.ReplicationSource{}
	if err := r.Client.Get(ctx, req.NamespacedName, inst); err != nil {
		if kerrors.IsNotFound(err) {
			logger.Error(err, "Failed to get Source")
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if inst.Status == nil {
		inst.Status = &volsyncv1alpha1.ReplicationSourceStatus{}
	}

	var result ctrl.Result
	var err error
	if r.countReplicationMethods(inst, logger) > 1 {
		err = fmt.Errorf("only a single replication method can be provided")
		return result, err
	}
	result, err = reconcileSrcUsingCatalog(ctx, inst, r, logger)
	if errors.Is(err, errNoMoverFound) {
		if inst.Spec.External != nil {
			// Not an internal method... we're done.
			return ctrl.Result{}, nil
		}
		err = fmt.Errorf("a replication method must be specified")
	}

	// Set reconcile status condition
	if err == nil {
		apimeta.SetStatusCondition(&inst.Status.Conditions, metav1.Condition{
			Type:    volsyncv1alpha1.ConditionReconciled,
			Status:  metav1.ConditionTrue,
			Reason:  volsyncv1alpha1.ReconciledReasonComplete,
			Message: "Reconcile complete",
		})
	} else {
		apimeta.SetStatusCondition(&inst.Status.Conditions, metav1.Condition{
			Type:    volsyncv1alpha1.ConditionReconciled,
			Status:  metav1.ConditionFalse,
			Reason:  volsyncv1alpha1.ReconciledReasonError,
			Message: err.Error(),
		})
	}

	// Update instance status
	statusErr := r.Client.Status().Update(ctx, inst)
	if err == nil { // Don't mask previous error
		err = statusErr
	}
	if !inst.Status.NextSyncTime.IsZero() {
		// ensure we get re-reconciled no later than the next scheduled sync
		// time
		delta := time.Until(inst.Status.NextSyncTime.Time)
		if delta > 0 {
			result.RequeueAfter = delta
		}
	}
	return result, err
}

var errNoMoverFound = fmt.Errorf("no matching data mover was found")

//nolint:funlen
func reconcileSrcUsingCatalog(
	ctx context.Context,
	instance *volsyncv1alpha1.ReplicationSource,
	sr *ReplicationSourceReconciler,
	logger logr.Logger,
) (ctrl.Result, error) {
	// Search the Mover catalog for a suitable data mover
	var dataMover mover.Mover
	for _, builder := range mover.Catalog {
		if candidate, err := builder.FromSource(sr.Client, logger, instance); err == nil && candidate != nil {
			if dataMover != nil {
				// Found 2 movers claiming this CR...
				return ctrl.Result{}, fmt.Errorf("only a single replication method can be provided")
			}
			dataMover = candidate
		}
	}
	if dataMover == nil { // No mover matched
		return ctrl.Result{}, errNoMoverFound
	}

	metrics := newVolSyncMetrics(prometheus.Labels{
		"obj_name":      instance.Name,
		"obj_namespace": instance.Namespace,
		"role":          "source",
		"method":        dataMover.Name(),
	})
	shouldSync, err := awaitNextSyncSource(instance, metrics, logger)
	if err != nil {
		return ctrl.Result{}, err
	}

	var mResult mover.Result
	if shouldSync && !apimeta.IsStatusConditionFalse(instance.Status.Conditions, volsyncv1alpha1.ConditionSynchronizing) {
		updateLastSyncStartTimeSource(instance) // Make sure lastSyncStartTime is set

		mResult, err = dataMover.Synchronize(ctx)
		if mResult.Completed {
			apimeta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:    volsyncv1alpha1.ConditionSynchronizing,
				Status:  metav1.ConditionFalse,
				Reason:  volsyncv1alpha1.SynchronizingReasonCleanup,
				Message: "Cleaning up",
			})
			if ok, err := updateLastSyncSource(instance, metrics, logger); !ok {
				return mover.InProgress().ReconcileResult(), err
			}
		}
	} else {
		mResult, err = dataMover.Cleanup(ctx)
		if mResult.Completed {
			apimeta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:    volsyncv1alpha1.ConditionSynchronizing,
				Status:  metav1.ConditionTrue,
				Reason:  volsyncv1alpha1.SynchronizingReasonSync,
				Message: "Synchronization in-progress",
			})
			// To update conditions
			_, _ = awaitNextSyncSource(instance, metrics, logger)
		}
	}
	return mResult.ReconcileResult(), err
}

func (r *ReplicationSourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&volsyncv1alpha1.ReplicationSource{}).
		Owns(&batchv1.Job{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Owns(&snapv1.VolumeSnapshot{}).
		Complete(r)
}

// pastScheduleDeadline returns true if a scheduled sync hasn't been completed
// within the synchronization period.
func pastScheduleDeadline(schedule cron.Schedule, lastCompleted time.Time, now time.Time) bool {
	// Each synchronization should complete before the next scheduled start
	// time. This means that, starting from the last completed, the next sync
	// would start at last->next, and must finish before last->next->next.
	return schedule.Next(schedule.Next(lastCompleted)).Before(now)
}

//nolint:dupl
func updateNextSyncSource(
	rs *volsyncv1alpha1.ReplicationSource,
	metrics volsyncMetrics,
	logger logr.Logger,
) (bool, error) {
	// if there's a schedule, and no manual trigger is set
	if rs.Spec.Trigger != nil &&
		rs.Spec.Trigger.Schedule != nil &&
		rs.Spec.Trigger.Manual == "" {
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
		schedule, err := parser.Parse(*rs.Spec.Trigger.Schedule)
		if err != nil {
			logger.Error(err, "error parsing schedule", "cronspec", rs.Spec.Trigger.Schedule)
			return false, err
		}

		// If we've previously completed a sync
		if !rs.Status.LastSyncTime.IsZero() {
			if pastScheduleDeadline(schedule, rs.Status.LastSyncTime.Time, time.Now()) {
				metrics.OutOfSync.Set(1)
			}
			next := schedule.Next(rs.Status.LastSyncTime.Time)
			rs.Status.NextSyncTime = &metav1.Time{Time: next}
		} else { // Never synced before, so we should ASAP
			rs.Status.NextSyncTime = &metav1.Time{Time: time.Now()}
		}
	} else { // No schedule, so there's no "next"
		rs.Status.NextSyncTime = nil
	}

	if rs.Status.LastSyncTime.IsZero() {
		// Never synced before, so we're out of sync
		metrics.OutOfSync.Set(1)
	}

	return true, nil
}

//nolint:funlen
func awaitNextSyncSource(
	rs *volsyncv1alpha1.ReplicationSource,
	metrics volsyncMetrics,
	logger logr.Logger,
) (bool, error) {
	// Ensure nextSyncTime is correct
	if cont, err := updateNextSyncSource(rs, metrics, logger); !cont || err != nil {
		return cont, err
	}

	// When manual trigger is set, but the lastManualSync value already matches
	// then we don't want to sync further.
	if rs.Spec.Trigger != nil && rs.Spec.Trigger.Manual != "" {
		if rs.Spec.Trigger.Manual == rs.Status.LastManualSync {
			apimeta.SetStatusCondition(&rs.Status.Conditions, metav1.Condition{
				Type:    volsyncv1alpha1.ConditionSynchronizing,
				Status:  metav1.ConditionFalse,
				Reason:  volsyncv1alpha1.SynchronizingReasonManual,
				Message: "Waiting for manual trigger",
			})
			return false, nil
		}
		apimeta.SetStatusCondition(&rs.Status.Conditions, metav1.Condition{
			Type:    volsyncv1alpha1.ConditionSynchronizing,
			Status:  metav1.ConditionTrue,
			Reason:  volsyncv1alpha1.SynchronizingReasonSync,
			Message: "Synchronization in-progress",
		})
		return true, nil
	}

	// If there's no schedule, (and no manual trigger), we should sync
	if rs.Status.NextSyncTime.IsZero() {
		// Condition update omitted intentionally to work with Mover inteface.
		return true, nil
	}

	// if it's past the nextSyncTime, we should sync
	if rs.Status.NextSyncTime.Time.Before(time.Now()) {
		apimeta.SetStatusCondition(&rs.Status.Conditions, metav1.Condition{
			Type:    volsyncv1alpha1.ConditionSynchronizing,
			Status:  metav1.ConditionTrue,
			Reason:  volsyncv1alpha1.SynchronizingReasonSync,
			Message: "Synchronization in-progress",
		})
		return true, nil
	}
	apimeta.SetStatusCondition(&rs.Status.Conditions, metav1.Condition{
		Type:    volsyncv1alpha1.ConditionSynchronizing,
		Status:  metav1.ConditionFalse,
		Reason:  volsyncv1alpha1.SynchronizingReasonSched,
		Message: "Waiting for next scheduled synchronization",
	})
	return false, nil
}

// Should be called only if synchronizing - will assume if lastSyncStartTime is not
// set that it should be set to now
func updateLastSyncStartTimeSource(rs *volsyncv1alpha1.ReplicationSource) {
	if rs.Status.LastSyncStartTime == nil {
		rs.Status.LastSyncStartTime = &metav1.Time{Time: time.Now()}
	}
}

//nolint:dupl
func updateLastSyncSource(
	rs *volsyncv1alpha1.ReplicationSource,
	metrics volsyncMetrics,
	logger logr.Logger,
) (bool, error) {
	// if there's a schedule see if we've made the deadline
	if rs.Spec.Trigger != nil && rs.Spec.Trigger.Schedule != nil && rs.Status.LastSyncTime != nil {
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
		schedule, err := parser.Parse(*rs.Spec.Trigger.Schedule)
		if err != nil {
			logger.Error(err, "error parsing schedule", "cronspec", rs.Spec.Trigger.Schedule)
			return false, err
		}
		if pastScheduleDeadline(schedule, rs.Status.LastSyncTime.Time, time.Now()) {
			metrics.MissedIntervals.Inc()
		} else {
			metrics.OutOfSync.Set(0)
		}
	} else {
		// There's no schedule or we just completed our first sync, so mark as
		// in-sync
		metrics.OutOfSync.Set(0)
	}

	rs.Status.LastSyncTime = &metav1.Time{Time: time.Now()}

	if rs.Status.LastSyncStartTime != nil {
		// Calculate sync duration
		d := rs.Status.LastSyncTime.Sub(rs.Status.LastSyncStartTime.Time)
		rs.Status.LastSyncDuration = &metav1.Duration{Duration: d}
		metrics.SyncDurations.Observe(d.Seconds())

		// Clear out lastSyncStartTime so next sync can set it again
		rs.Status.LastSyncStartTime = nil
	}

	// When a sync is completed we set lastManualSync
	if rs.Spec.Trigger != nil {
		rs.Status.LastManualSync = rs.Spec.Trigger.Manual
	} else {
		rs.Status.LastManualSync = ""
	}

	return updateNextSyncSource(rs, metrics, logger)
}
