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
	"github.com/backube/volsync/controllers/utils"
)

// ReplicationDestinationReconciler reconciles a ReplicationDestination object
type ReplicationDestinationReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

//nolint:lll
//+kubebuilder:rbac:groups=volsync.backube,resources=replicationdestinations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=volsync.backube,resources=replicationdestinations/finalizers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=volsync.backube,resources=replicationdestinations/status,verbs=get;update;patch
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
func (r *ReplicationDestinationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("replicationdestination", req.NamespacedName)
	// Get CR instance
	inst := &volsyncv1alpha1.ReplicationDestination{}
	if err := r.Client.Get(ctx, req.NamespacedName, inst); err != nil {
		if !kerrors.IsNotFound(err) {
			logger.Error(err, "Failed to get Destination")
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	// Prepare the .Status fields if necessary
	if inst.Status == nil {
		inst.Status = &volsyncv1alpha1.ReplicationDestinationStatus{}
	}

	var result ctrl.Result
	var err error
	if r.countReplicationMethods(inst, logger) > 1 {
		err = fmt.Errorf("only a single replication method can be provided")
		return result, err
	}
	result, err = reconcileDestUsingCatalog(ctx, inst, r, logger)
	if errors.Is(err, errNoMoverFound) { // do the old stuff
		// Not returning error at this point - preserves External as a possible RS Spec //TODO: is this correct?
		// Not an internal method... we're done.
		return ctrl.Result{}, nil
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

//nolint:funlen
func reconcileDestUsingCatalog(
	ctx context.Context,
	instance *volsyncv1alpha1.ReplicationDestination,
	dr *ReplicationDestinationReconciler,
	logger logr.Logger,
) (ctrl.Result, error) {
	// Search the Mover catalog for a suitable data mover
	var dataMover mover.Mover
	for _, builder := range mover.Catalog {
		if candidate, err := builder.FromDestination(dr.Client, logger, instance); err == nil && candidate != nil {
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
		"role":          "destination",
		"method":        dataMover.Name(),
	})

	shouldSync, err := awaitNextSyncDestination(instance, metrics, logger)
	if err != nil {
		return ctrl.Result{}, err
	}

	var result mover.Result
	if shouldSync && !apimeta.IsStatusConditionFalse(instance.Status.Conditions, volsyncv1alpha1.ConditionSynchronizing) {
		updateLastSyncStartTimeDestination(instance) // Make sure lastSyncStartTime is set

		result, err = dataMover.Synchronize(ctx)
		if result.Completed && result.Image != nil {
			// Mark previous latestImage for cleanup if it was a snapshot
			err = utils.MarkOldSnapshotForCleanup(ctx, dr.Client, logger, instance,
				instance.Status.LatestImage, result.Image)
			if err != nil {
				return mover.InProgress().ReconcileResult(), err
			}

			instance.Status.LatestImage = result.Image
			apimeta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:    volsyncv1alpha1.ConditionSynchronizing,
				Status:  metav1.ConditionFalse,
				Reason:  volsyncv1alpha1.SynchronizingReasonCleanup,
				Message: "Cleaning up",
			})
			if ok, err := updateLastSyncDestination(instance, metrics, logger); !ok {
				return mover.InProgress().ReconcileResult(), err
			}
		}
	} else {
		result, err = dataMover.Cleanup(ctx)
		if result.Completed {
			apimeta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:    volsyncv1alpha1.ConditionSynchronizing,
				Status:  metav1.ConditionTrue,
				Reason:  volsyncv1alpha1.SynchronizingReasonSync,
				Message: "Synchronization in-progress",
			})
			// To update conditions
			_, _ = awaitNextSyncDestination(instance, metrics, logger)
		}
	}
	return result.ReconcileResult(), err
}

func (r *ReplicationDestinationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&volsyncv1alpha1.ReplicationDestination{}).
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

//nolint:dupl
func updateNextSyncDestination(
	rd *volsyncv1alpha1.ReplicationDestination,
	metrics volsyncMetrics,
	logger logr.Logger,
) (bool, error) {
	// if there's a schedule, and no manual trigger is set
	if rd.Spec.Trigger != nil &&
		rd.Spec.Trigger.Schedule != nil &&
		rd.Spec.Trigger.Manual == "" {
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
		schedule, err := parser.Parse(*rd.Spec.Trigger.Schedule)
		if err != nil {
			logger.Error(err, "error parsing schedule", "cronspec", rd.Spec.Trigger.Schedule)
			return false, err
		}

		// If we've previously completed a sync
		if !rd.Status.LastSyncTime.IsZero() {
			if pastScheduleDeadline(schedule, rd.Status.LastSyncTime.Time, time.Now()) {
				metrics.OutOfSync.Set(1)
			}
			next := schedule.Next(rd.Status.LastSyncTime.Time)
			rd.Status.NextSyncTime = &metav1.Time{Time: next}
		} else { // Never synced before, so we should ASAP
			rd.Status.NextSyncTime = &metav1.Time{Time: time.Now()}
		}
	} else { // No schedule, so there's no "next"
		rd.Status.NextSyncTime = nil
	}

	if rd.Status.LastSyncTime.IsZero() {
		// Never synced before, so we're out of sync
		metrics.OutOfSync.Set(1)
	}

	return true, nil
}

//nolint:funlen
func awaitNextSyncDestination(
	rd *volsyncv1alpha1.ReplicationDestination,
	metrics volsyncMetrics,
	logger logr.Logger,
) (bool, error) {
	// Ensure nextSyncTime is correct
	if cont, err := updateNextSyncDestination(rd, metrics, logger); !cont || err != nil {
		return cont, err
	}

	// When manual trigger is set, but the lastManualSync value already matches
	// then we don't want to sync further.
	if rd.Spec.Trigger != nil && rd.Spec.Trigger.Manual != "" {
		if rd.Spec.Trigger.Manual == rd.Status.LastManualSync {
			apimeta.SetStatusCondition(&rd.Status.Conditions, metav1.Condition{
				Type:    volsyncv1alpha1.ConditionSynchronizing,
				Status:  metav1.ConditionFalse,
				Reason:  volsyncv1alpha1.SynchronizingReasonManual,
				Message: "Waiting for manual trigger",
			})
			return false, nil
		}
		apimeta.SetStatusCondition(&rd.Status.Conditions, metav1.Condition{
			Type:    volsyncv1alpha1.ConditionSynchronizing,
			Status:  metav1.ConditionTrue,
			Reason:  volsyncv1alpha1.SynchronizingReasonSync,
			Message: "Synchronization in-progress",
		})
		return true, nil
	}

	// If there's no schedule, (and no manual trigger), we should sync
	if rd.Status.NextSyncTime.IsZero() {
		// Condition update omitted intentionally to work with Mover inteface.
		return true, nil
	}

	// if it's past the nextSyncTime, we should sync
	if rd.Status.NextSyncTime.Time.Before(time.Now()) {
		apimeta.SetStatusCondition(&rd.Status.Conditions, metav1.Condition{
			Type:    volsyncv1alpha1.ConditionSynchronizing,
			Status:  metav1.ConditionTrue,
			Reason:  volsyncv1alpha1.SynchronizingReasonSync,
			Message: "Synchronization in-progress",
		})
		return true, nil
	}
	apimeta.SetStatusCondition(&rd.Status.Conditions, metav1.Condition{
		Type:    volsyncv1alpha1.ConditionSynchronizing,
		Status:  metav1.ConditionFalse,
		Reason:  volsyncv1alpha1.SynchronizingReasonSched,
		Message: "Waiting for next scheduled synchronization",
	})
	return false, nil
}

// Should be called only if synchronizing - will assume if lastSyncStartTime is not
// set that it should be set to now
func updateLastSyncStartTimeDestination(rs *volsyncv1alpha1.ReplicationDestination) {
	if rs.Status.LastSyncStartTime == nil {
		rs.Status.LastSyncStartTime = &metav1.Time{Time: time.Now()}
	}
}

//nolint:dupl
func updateLastSyncDestination(
	rd *volsyncv1alpha1.ReplicationDestination,
	metrics volsyncMetrics,
	logger logr.Logger,
) (bool, error) {
	// if there's a schedule see if we've made the deadline
	if rd.Spec.Trigger != nil && rd.Spec.Trigger.Schedule != nil && rd.Status.LastSyncTime != nil {
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
		schedule, err := parser.Parse(*rd.Spec.Trigger.Schedule)
		if err != nil {
			logger.Error(err, "error parsing schedule", "cronspec", rd.Spec.Trigger.Schedule)
			return false, err
		}
		if pastScheduleDeadline(schedule, rd.Status.LastSyncTime.Time, time.Now()) {
			metrics.MissedIntervals.Inc()
		} else {
			metrics.OutOfSync.Set(0)
		}
	} else {
		// There's no schedule or we just completed our first sync, so mark as
		// in-sync
		metrics.OutOfSync.Set(0)
	}

	rd.Status.LastSyncTime = &metav1.Time{Time: time.Now()}

	if rd.Status.LastSyncStartTime != nil {
		// Calculate sync duration
		d := rd.Status.LastSyncTime.Sub(rd.Status.LastSyncStartTime.Time)
		rd.Status.LastSyncDuration = &metav1.Duration{Duration: d}
		metrics.SyncDurations.Observe(d.Seconds())

		// Clear out lastSyncStartTime so next sync can set it again
		rd.Status.LastSyncStartTime = nil
	}

	// When a sync is completed we set lastManualSync
	if rd.Spec.Trigger != nil {
		rd.Status.LastManualSync = rd.Spec.Trigger.Manual
	} else {
		rd.Status.LastManualSync = ""
	}

	return updateNextSyncDestination(rd, metrics, logger)
}
