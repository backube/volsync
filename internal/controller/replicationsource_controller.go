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

package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/prometheus/client_golang/prometheus"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/internal/controller/mover"
	sm "github.com/backube/volsync/internal/controller/statemachine"
	"github.com/backube/volsync/internal/controller/utils"
)

const (
	ReplicationSourceToSourcePVCIndex string = "replicationsource.spec.sourcePVC"
)

// ReplicationSourceReconciler reconciles a ReplicationSource object
type ReplicationSourceReconciler struct {
	client.Client
	Log           logr.Logger
	Scheme        *runtime.Scheme
	EventRecorder record.EventRecorder
}

type rsMachine struct {
	rs      *volsyncv1alpha1.ReplicationSource
	client  client.Client
	logger  logr.Logger
	metrics volsyncMetrics
	mover   mover.Mover
}

var _ sm.ReplicationMachine = &rsMachine{}

//nolint:lll
//nolint:funlen
//+kubebuilder:rbac:groups=volsync.backube,resources=replicationsources,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=volsync.backube,resources=replicationsources/finalizers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=volsync.backube,resources=replicationsources/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete;deletecollection
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;update;patch
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete;deletecollection
//+kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete;deletecollection
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch
//+kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=create;update;patch
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,resourceNames=volsync-privileged-mover,verbs=use
//+kubebuilder:rbac:groups=snapshot.storage.k8s.io,resources=volumesnapshots,verbs=get;list;watch;create;update;patch;delete;deletecollection

//nolint:funlen
func (r *ReplicationSourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("replicationsource", req.NamespacedName)
	inst := &volsyncv1alpha1.ReplicationSource{}
	if err := r.Get(ctx, req.NamespacedName, inst); err != nil {
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

	// Handle deletion - check if resource is being deleted
	if !inst.GetDeletionTimestamp().IsZero() {
		logger.Info("ReplicationSource is being deleted, starting cleanup")
		return r.handleDeletion(ctx, inst, logger)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(inst, volsyncv1alpha1.ReplicationSourceFinalizer) {
		controllerutil.AddFinalizer(inst, volsyncv1alpha1.ReplicationSourceFinalizer)
		if err := r.Update(ctx, inst); err != nil {
			if kerrors.IsConflict(err) {
				return ctrl.Result{RequeueAfter: time.Second}, nil
			}
			logger.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		logger.Info("Added finalizer to ReplicationSource")
		// Requeue to continue with normal processing after adding finalizer
		return ctrl.Result{Requeue: true}, nil
	}

	// Check if privileged movers are allowed via namespace annotation
	privilegedMoverOk, err := utils.PrivilegedMoversOk(ctx, r.Client, logger, inst.GetNamespace())
	if err != nil {
		return result, err
	}

	rsm, err := newRSMachine(inst, r.Client, logger,
		record.NewEventRecorderAdapter(mover.NewEventRecorderLogger(r.EventRecorder)), privilegedMoverOk)

	// Using only external method
	if errors.Is(err, mover.ErrNoMoverFound) && inst.Spec.External != nil {
		return ctrl.Result{}, nil
	}
	// Both internal and external methods defined
	if rsm != nil && inst.Spec.External != nil {
		err = mover.ErrMultipleMoversFound
		apimeta.SetStatusCondition(&inst.Status.Conditions, metav1.Condition{
			Type:    volsyncv1alpha1.ConditionSynchronizing,
			Status:  metav1.ConditionFalse,
			Reason:  volsyncv1alpha1.SynchronizingReasonError,
			Message: err.Error(),
		})
	}
	// No method found
	if rsm == nil && inst.Spec.External == nil {
		err = mover.ErrNoMoverFound
		apimeta.SetStatusCondition(&inst.Status.Conditions, metav1.Condition{
			Type:    volsyncv1alpha1.ConditionSynchronizing,
			Status:  metav1.ConditionFalse,
			Reason:  volsyncv1alpha1.SynchronizingReasonError,
			Message: err.Error() + fmt.Sprintf(" - enabled movers: %v", mover.GetEnabledMoverList()),
		})
	}

	// All good, so run the state machine
	if err == nil {
		result, err = sm.Run(ctx, rsm, logger)
	}

	// Update instance status
	statusErr := r.Client.Status().Update(ctx, inst)
	if err == nil { // Don't mask previous error
		err = statusErr
	}
	return result, err
}

// handleDeletion handles the deletion of a ReplicationSource by cleaning up owned resources
// and removing the finalizer once cleanup is complete
func (r *ReplicationSourceReconciler) handleDeletion(ctx context.Context,
	inst *volsyncv1alpha1.ReplicationSource, logger logr.Logger) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(inst, volsyncv1alpha1.ReplicationSourceFinalizer) {
		// Finalizer not present, nothing to clean up
		return ctrl.Result{}, nil
	}

	logger.Info("Cleaning up ReplicationSource resources")

	// Perform mover-specific cleanup
	result, err := r.cleanupMoverResources(ctx, inst, logger)
	if err != nil {
		return result, err
	}
	if result.Requeue || result.RequeueAfter > 0 {
		return result, nil
	}

	// Clean up remaining owned resources
	if err := r.cleanupOwnedResources(ctx, inst, logger); err != nil {
		return r.handleCleanupError(ctx, inst, logger, err, "Resource cleanup error during deletion")
	}

	// Remove finalizer and complete deletion
	return r.removeFinalizer(ctx, inst, logger)
}

// cleanupMoverResources handles cleanup of mover-specific resources
func (r *ReplicationSourceReconciler) cleanupMoverResources(ctx context.Context,
	inst *volsyncv1alpha1.ReplicationSource, logger logr.Logger) (ctrl.Result, error) {
	// Use shared utility function with pre-built source mover factory
	moverFactory := utils.CreateReplicationSourceMoverFactory()
	eventRecorder := record.NewEventRecorderAdapter(mover.NewEventRecorderLogger(r.EventRecorder))
	result, err := utils.CleanupMoverResources(ctx, inst, r.Client, logger, eventRecorder, moverFactory)
	if err != nil {
		return r.handleCleanupError(ctx, inst, logger, err, "Cleanup error during deletion")
	}
	return result, nil
}

// cleanupOwnedResources cleans up owned Kubernetes resources
func (r *ReplicationSourceReconciler) cleanupOwnedResources(ctx context.Context,
	inst *volsyncv1alpha1.ReplicationSource, logger logr.Logger) error {
	cleanupTypes := []client.Object{
		&batchv1.Job{},
		&corev1.PersistentVolumeClaim{},
		&corev1.Secret{},
		&corev1.Service{},
		&corev1.ServiceAccount{},
		&rbacv1.Role{},
		&rbacv1.RoleBinding{},
		&snapv1.VolumeSnapshot{},
	}

	return utils.CleanupObjects(ctx, r.Client, logger, inst, cleanupTypes)
}

// handleCleanupError handles cleanup errors by updating status and requeuing
func (r *ReplicationSourceReconciler) handleCleanupError(ctx context.Context,
	inst *volsyncv1alpha1.ReplicationSource, logger logr.Logger, err error, message string) (ctrl.Result, error) {
	logger.Error(err, message)
	// Update status to indicate cleanup error but don't fail deletion
	apimeta.SetStatusCondition(&inst.Status.Conditions, metav1.Condition{
		Type:    volsyncv1alpha1.ConditionSynchronizing,
		Status:  metav1.ConditionFalse,
		Reason:  volsyncv1alpha1.SynchronizingReasonCleanup,
		Message: fmt.Sprintf("%s: %v", message, err),
	})
	// Update status and requeue to retry cleanup
	if statusErr := r.Client.Status().Update(ctx, inst); statusErr != nil {
		logger.Error(statusErr, "Failed to update status during cleanup")
	}
	return ctrl.Result{RequeueAfter: time.Second * 30}, nil
}

// removeFinalizer removes the finalizer and completes the deletion process
func (r *ReplicationSourceReconciler) removeFinalizer(ctx context.Context,
	inst *volsyncv1alpha1.ReplicationSource, logger logr.Logger) (ctrl.Result, error) {
	controllerutil.RemoveFinalizer(inst, volsyncv1alpha1.ReplicationSourceFinalizer)
	if err := r.Update(ctx, inst); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	logger.Info("ReplicationSource cleanup completed successfully")
	return ctrl.Result{}, nil
}

func (r *ReplicationSourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&volsyncv1alpha1.ReplicationSource{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 100,
		}).
		Owns(&batchv1.Job{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Owns(&snapv1.VolumeSnapshot{}).
		Watches(&corev1.PersistentVolumeClaim{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, o client.Object) []reconcile.Request {
				return mapFuncCopyTriggerPVCToReplicationSource(ctx, mgr.GetClient(), o)
			}), builder.WithPredicates(copyTriggerPVCPredicate())).
		Complete(r)
}

func mapFuncCopyTriggerPVCToReplicationSource(ctx context.Context, k8sClient client.Client,
	o client.Object) []reconcile.Request {
	logger := ctrl.Log.WithName("mapFuncCopyTriggerPVCToReplicationSource")

	pvc, ok := o.(*corev1.PersistentVolumeClaim)
	if !ok {
		return []reconcile.Request{}
	}

	// Only continue if the PVC is using the use-copy-trigger annotation
	if !utils.PVCUsesCopyTrigger(pvc) {
		return []reconcile.Request{}
	}

	// Find if we have any ReplicationSources using this PVC as a source
	// This will break if multiple replicationsources use the same PVC as a source
	rsList := &volsyncv1alpha1.ReplicationSourceList{}
	err := k8sClient.List(ctx, rsList,
		client.MatchingFields{ReplicationSourceToSourcePVCIndex: pvc.GetName()}, // custom index
		client.InNamespace(pvc.GetNamespace()))
	if err != nil {
		logger.Error(err, "Error looking up replicationsources (using index) matching source PVC",
			"pvc name", pvc.GetName(), "namespace", pvc.GetNamespace(),
			"index name", ReplicationSourceToSourcePVCIndex)
		return []reconcile.Request{}
	}

	reqs := []reconcile.Request{}
	for i := range rsList.Items {
		rs := rsList.Items[i]
		reqs = append(reqs, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      rs.GetName(),
				Namespace: rs.GetNamespace(),
			},
		})
	}

	return reqs
}

func copyTriggerPVCPredicate() predicate.Predicate {
	// Only reconcile ReplicationSources for PVC if the PVC is new or updated (no delete)
	return predicate.Funcs{
		CreateFunc: func(_ event.CreateEvent) bool {
			return true
		},
		DeleteFunc: func(_ event.DeleteEvent) bool {
			return false
		},
		UpdateFunc: func(_ event.UpdateEvent) bool {
			return true
		},
		GenericFunc: func(_ event.GenericEvent) bool {
			return true
		},
	}
}

func IndexFieldsForReplicationSource(ctx context.Context, fieldIndexer client.FieldIndexer) error {
	// Index on ReplicationSources - used to find ReplicationSources with SourcePVC referring to a PVC
	return fieldIndexer.IndexField(ctx, &volsyncv1alpha1.ReplicationSource{},
		ReplicationSourceToSourcePVCIndex, func(o client.Object) []string {
			var res []string
			replicationSource, ok := o.(*volsyncv1alpha1.ReplicationSource)
			if !ok {
				// This shouldn't happen
				return res
			}

			// just return the raw field value -- the indexer will take care of dealing with namespaces for us
			sourcePVC := replicationSource.Spec.SourcePVC
			if sourcePVC != "" {
				res = append(res, sourcePVC)
			}
			return res
		})
}

func newRSMachine(rs *volsyncv1alpha1.ReplicationSource, c client.Client,
	l logr.Logger, er events.EventRecorder, privilegedMoverOk bool) (*rsMachine, error) {
	dataMover, err := mover.GetSourceMoverFromCatalog(c, l, er, rs, privilegedMoverOk)
	if err != nil {
		return nil, err
	}

	metrics := newVolSyncMetrics(prometheus.Labels{
		"obj_name":      rs.Name,
		"obj_namespace": rs.Namespace,
		"role":          "source",
		"method":        dataMover.Name(),
	})

	return &rsMachine{
		rs:      rs,
		client:  c,
		logger:  l,
		metrics: metrics,
		mover:   dataMover,
	}, nil
}

func (m *rsMachine) Cronspec() string {
	if m.rs.Spec.Trigger != nil && m.rs.Spec.Trigger.Schedule != nil {
		return *m.rs.Spec.Trigger.Schedule
	}
	return ""
}

func (m *rsMachine) ManualTag() string {
	if m.rs.Spec.Trigger != nil {
		return m.rs.Spec.Trigger.Manual
	}
	return ""
}

func (m *rsMachine) LastManualTag() string {
	return m.rs.Status.LastManualSync
}

func (m *rsMachine) SetLastManualTag(tag string) {
	m.rs.Status.LastManualSync = tag
}

func (m *rsMachine) NextSyncTime() *metav1.Time {
	return m.rs.Status.NextSyncTime
}

func (m *rsMachine) SetNextSyncTime(next *metav1.Time) {
	m.rs.Status.NextSyncTime = next
}

func (m *rsMachine) LastSyncStartTime() *metav1.Time {
	return m.rs.Status.LastSyncStartTime
}

func (m *rsMachine) SetLastSyncStartTime(last *metav1.Time) {
	m.rs.Status.LastSyncStartTime = last
}

func (m *rsMachine) LastSyncTime() *metav1.Time {
	return m.rs.Status.LastSyncTime
}

func (m *rsMachine) SetLastSyncTime(last *metav1.Time) {
	m.rs.Status.LastSyncTime = last
}

func (m *rsMachine) LastSyncDuration() *metav1.Duration {
	return m.rs.Status.LastSyncDuration
}

func (m *rsMachine) SetLastSyncDuration(duration *metav1.Duration) {
	m.rs.Status.LastSyncDuration = duration
}

func (m *rsMachine) Conditions() *[]metav1.Condition {
	return &m.rs.Status.Conditions
}

func (m *rsMachine) SetOutOfSync(isOutOfSync bool) {
	if isOutOfSync {
		m.metrics.OutOfSync.Set(1)
	} else {
		m.metrics.OutOfSync.Set(0)
	}
}

func (m *rsMachine) IncMissedIntervals() {
	m.metrics.MissedIntervals.Inc()
}

func (m *rsMachine) ObserveSyncDuration(duration time.Duration) {
	m.metrics.SyncDurations.Observe(duration.Seconds())
}

func (m *rsMachine) Synchronize(ctx context.Context) (mover.Result, error) {
	return m.mover.Synchronize(ctx)
}

func (m *rsMachine) Cleanup(ctx context.Context) (mover.Result, error) {
	return m.mover.Cleanup(ctx)
}
