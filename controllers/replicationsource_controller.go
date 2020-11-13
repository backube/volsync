/*
Copyright 2020 The Scribe authors.

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

	"github.com/go-logr/logr"
	"github.com/operator-framework/operator-lib/status"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	scribev1alpha1 "github.com/backube/scribe/api/v1alpha1"
)

// ReplicationSourceReconciler reconciles a ReplicationSource object
type ReplicationSourceReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=scribe.backube,resources=replicationsources,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=scribe.backube,resources=replicationsources/status,verbs=get;update;patch

func (r *ReplicationSourceReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	logger := r.Log.WithValues("replicationsource", req.NamespacedName)

	inst := &scribev1alpha1.ReplicationSource{}
	if err := r.Client.Get(ctx, req.NamespacedName, inst); err != nil {
		if kerrors.IsNotFound(err) {
			logger.Error(err, "Failed to get Source")
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if inst.Status == nil {
		inst.Status = &scribev1alpha1.ReplicationSourceStatus{}
	}
	if inst.Status.Conditions == nil {
		inst.Status.Conditions = status.Conditions{}
	}

	var result ctrl.Result
	var err error
	if inst.Spec.Rsync != nil {
		result, err = RunRsyncSrcReconciler(ctx, inst, r, logger)
	} else {
		return ctrl.Result{}, nil
	}

	// Set reconcile status condition
	if err == nil {
		inst.Status.Conditions.SetCondition(
			status.Condition{
				Type:    scribev1alpha1.ConditionReconciled,
				Status:  corev1.ConditionTrue,
				Reason:  scribev1alpha1.ReconciledReasonComplete,
				Message: "Reconcile complete",
			})
	} else {
		inst.Status.Conditions.SetCondition(
			status.Condition{
				Type:    scribev1alpha1.ConditionReconciled,
				Status:  corev1.ConditionFalse,
				Reason:  scribev1alpha1.ReconciledReasonError,
				Message: err.Error(),
			})
	}

	// Update instance status
	statusErr := r.Client.Status().Update(ctx, inst)
	if err == nil { // Don't mask previous error
		err = statusErr
	}
	return result, err
}

func (r *ReplicationSourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&scribev1alpha1.ReplicationSource{}).
		Complete(r)
}

type rsyncSrcReconciler struct {
	sourceVolumeHandler
}

func RunRsyncSrcReconciler(ctx context.Context, instance *scribev1alpha1.ReplicationSource,
	sr *ReplicationSourceReconciler, logger logr.Logger) (ctrl.Result, error) {
	r := rsyncSrcReconciler{
		sourceVolumeHandler: sourceVolumeHandler{
			Ctx:                         ctx,
			Instance:                    instance,
			ReplicationSourceReconciler: *sr,
			Options:                     &instance.Spec.Rsync.ReplicationSourceVolumeOptions,
		},
	}

	l := logger.WithValues("method", "Rsync")

	// Make sure there's a place to write status info
	if r.Instance.Status.Rsync == nil {
		r.Instance.Status.Rsync = &scribev1alpha1.ReplicationSourceRsyncStatus{}
	}

	_, err := reconcileBatch(l,
		r.EnsurePVC,
	)
	return ctrl.Result{}, err
}
