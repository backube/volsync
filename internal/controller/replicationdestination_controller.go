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
	"k8s.io/client-go/tools/events"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/internal/controller/mover"
	sm "github.com/backube/volsync/internal/controller/statemachine"
	"github.com/backube/volsync/internal/controller/utils"
)

// ReplicationDestinationReconciler reconciles a ReplicationDestination object
type ReplicationDestinationReconciler struct {
	client.Client
	Log           logr.Logger
	Scheme        *runtime.Scheme
	EventRecorder record.EventRecorder
}

type rdMachine struct {
	rd      *volsyncv1alpha1.ReplicationDestination
	client  client.Client
	logger  logr.Logger
	metrics volsyncMetrics
	mover   mover.Mover
}

var _ sm.ReplicationMachine = &rdMachine{}

//nolint:lll
//+kubebuilder:rbac:groups=volsync.backube,resources=replicationdestinations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=volsync.backube,resources=replicationdestinations/finalizers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=volsync.backube,resources=replicationdestinations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete;deletecollection
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;update;patch
//+kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete;deletecollection
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
func (r *ReplicationDestinationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("replicationdestination", req.NamespacedName)
	// Get CR instance
	inst := &volsyncv1alpha1.ReplicationDestination{}
	if err := r.Get(ctx, req.NamespacedName, inst); err != nil {
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

	// Check if any volume snapshots are marked with do-not-delete label and remove ownership if so
	err = utils.RelinquishOwnedSnapshotsWithDoNotDeleteLabel(ctx, r.Client, logger, inst)
	if err != nil {
		return result, err
	}

	// Check if privileged movers are allowed via namespace annotation
	privilegedMoverOk, err := utils.PrivilegedMoversOk(ctx, r.Client, logger, inst.GetNamespace())
	if err != nil {
		return result, err
	}

	rdm, err := newRDMachine(inst, r.Client, logger,
		record.NewEventRecorderAdapter(mover.NewEventRecorderLogger(r.EventRecorder)), privilegedMoverOk)

	// Using only external method
	if errors.Is(err, mover.ErrNoMoverFound) && inst.Spec.External != nil {
		return ctrl.Result{}, nil
	}
	// Both internal and external methods defined
	if rdm != nil && inst.Spec.External != nil {
		err = mover.ErrMultipleMoversFound
		apimeta.SetStatusCondition(&inst.Status.Conditions, metav1.Condition{
			Type:    volsyncv1alpha1.ConditionSynchronizing,
			Status:  metav1.ConditionFalse,
			Reason:  volsyncv1alpha1.SynchronizingReasonError,
			Message: err.Error(),
		})
	}
	// No method found
	if rdm == nil && inst.Spec.External == nil {
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
		result, err = sm.Run(ctx, rdm, logger)
	}

	// Update instance status
	statusErr := r.Client.Status().Update(ctx, inst)
	if err == nil { // Don't mask previous error
		err = statusErr
	}
	return result, err
}

func (r *ReplicationDestinationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&volsyncv1alpha1.ReplicationDestination{}).
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
		Complete(r)
}

func newRDMachine(rd *volsyncv1alpha1.ReplicationDestination, c client.Client,
	l logr.Logger, er events.EventRecorder, privilegedMoverOk bool) (*rdMachine, error) {
	dataMover, err := mover.GetDestinationMoverFromCatalog(c, l, er, rd, privilegedMoverOk)
	if err != nil {
		return nil, err
	}

	metrics := newVolSyncMetrics(prometheus.Labels{
		"obj_name":      rd.Name,
		"obj_namespace": rd.Namespace,
		"role":          "destination",
		"method":        dataMover.Name(),
	})

	return &rdMachine{
		rd:      rd,
		client:  c,
		logger:  l,
		metrics: metrics,
		mover:   dataMover,
	}, nil
}

func (m *rdMachine) Cronspec() string {
	if m.rd.Spec.Trigger != nil && m.rd.Spec.Trigger.Schedule != nil {
		return *m.rd.Spec.Trigger.Schedule
	}
	return ""
}

func (m *rdMachine) ManualTag() string {
	if m.rd.Spec.Trigger != nil {
		return m.rd.Spec.Trigger.Manual
	}
	return ""
}

func (m *rdMachine) LastManualTag() string {
	return m.rd.Status.LastManualSync
}

func (m *rdMachine) SetLastManualTag(tag string) {
	m.rd.Status.LastManualSync = tag
}

func (m *rdMachine) NextSyncTime() *metav1.Time {
	return m.rd.Status.NextSyncTime
}

func (m *rdMachine) SetNextSyncTime(next *metav1.Time) {
	m.rd.Status.NextSyncTime = next
}

func (m *rdMachine) LastSyncStartTime() *metav1.Time {
	return m.rd.Status.LastSyncStartTime
}

func (m *rdMachine) SetLastSyncStartTime(last *metav1.Time) {
	m.rd.Status.LastSyncStartTime = last
}

func (m *rdMachine) LastSyncTime() *metav1.Time {
	return m.rd.Status.LastSyncTime
}

func (m *rdMachine) SetLastSyncTime(last *metav1.Time) {
	m.rd.Status.LastSyncTime = last
}

func (m *rdMachine) LastSyncDuration() *metav1.Duration {
	return m.rd.Status.LastSyncDuration
}

func (m *rdMachine) SetLastSyncDuration(duration *metav1.Duration) {
	m.rd.Status.LastSyncDuration = duration
}

func (m *rdMachine) Conditions() *[]metav1.Condition {
	return &m.rd.Status.Conditions
}

func (m *rdMachine) SetOutOfSync(isOutOfSync bool) {
	if isOutOfSync {
		m.metrics.OutOfSync.Set(1)
	} else {
		m.metrics.OutOfSync.Set(0)
	}
}

func (m *rdMachine) IncMissedIntervals() {
	m.metrics.MissedIntervals.Inc()
}

func (m *rdMachine) ObserveSyncDuration(duration time.Duration) {
	m.metrics.SyncDurations.Observe(duration.Seconds())
}

func (m *rdMachine) Synchronize(ctx context.Context) (mover.Result, error) {
	result, err := m.mover.Synchronize(ctx)

	if result.Completed && result.Image != nil {
		// Mark previous latestImage for cleanup if it was a snapshot
		err = utils.MarkOldSnapshotForCleanup(ctx, m.client, m.logger, m.rd,
			m.rd.Status.LatestImage, result.Image)
		if err != nil {
			return mover.InProgress(), err
		}

		m.rd.Status.LatestImage = result.Image
	}

	return result, err
}

func (m *rdMachine) Cleanup(ctx context.Context) (mover.Result, error) {
	return m.mover.Cleanup(ctx)
}
