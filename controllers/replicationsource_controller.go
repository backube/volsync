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
	"strconv"
	"time"

	"github.com/go-logr/logr"
	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1beta1"
	"github.com/operator-framework/operator-lib/status"
	"github.com/prometheus/client_golang/prometheus"
	cron "github.com/robfig/cron/v3"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/controllers/mover"
	"github.com/backube/volsync/controllers/utils"
)

const (
	// mountPath is the source and destination data directory location
	mountPath = "/data"
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
	if inst.Status.Conditions == nil {
		inst.Status.Conditions = status.Conditions{}
	}

	var result ctrl.Result
	var err error
	if r.countReplicationMethods(inst, logger) > 1 {
		err = fmt.Errorf("only a single replication method can be provided")
		return result, err
	}
	result, err = reconcileSrcUsingCatalog(ctx, inst, r, logger)
	if errors.Is(err, errNoMoverFound) { // do the old stuff
		if inst.Spec.Rsync != nil {
			result, err = RunRsyncSrcReconciler(ctx, inst, r, logger)
		} else if inst.Spec.Rclone != nil {
			result, err = RunRcloneSrcReconciler(ctx, inst, r, logger)
		} else {
			return ctrl.Result{}, nil
		}
	}

	// Set reconcile status condition
	if err == nil {
		inst.Status.Conditions.SetCondition(
			status.Condition{
				Type:    volsyncv1alpha1.ConditionReconciled,
				Status:  corev1.ConditionTrue,
				Reason:  volsyncv1alpha1.ReconciledReasonComplete,
				Message: "Reconcile complete",
			})
	} else {
		inst.Status.Conditions.SetCondition(
			status.Condition{
				Type:    volsyncv1alpha1.ConditionReconciled,
				Status:  corev1.ConditionFalse,
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
		if candidate, err := builder.FromSource(sr.Client, logger, instance); err == nil {
			if dataMover != nil && candidate != nil {
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
	if shouldSync && !instance.Status.Conditions.IsFalseFor(volsyncv1alpha1.ConditionSynchronizing) {
		mResult, err = dataMover.Synchronize(ctx)
		if mResult.Completed {
			instance.Status.Conditions.SetCondition(
				status.Condition{
					Type:    volsyncv1alpha1.ConditionSynchronizing,
					Status:  corev1.ConditionFalse,
					Reason:  volsyncv1alpha1.SynchronizingReasonCleanup,
					Message: "Cleaning up",
				},
			)
			if ok, err := updateLastSyncSource(instance, metrics, logger); !ok {
				return mover.InProgress().ReconcileResult(), err
			}
		}
	} else {
		mResult, err = dataMover.Cleanup(ctx)
		if mResult.Completed {
			instance.Status.Conditions.SetCondition(
				status.Condition{
					Type:    volsyncv1alpha1.ConditionSynchronizing,
					Status:  corev1.ConditionTrue,
					Reason:  volsyncv1alpha1.SynchronizingReasonSync,
					Message: "Synchronization in-progress",
				},
			)
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
			rs.Status.Conditions.SetCondition(
				status.Condition{
					Type:    volsyncv1alpha1.ConditionSynchronizing,
					Status:  corev1.ConditionFalse,
					Reason:  volsyncv1alpha1.SynchronizingReasonManual,
					Message: "Waiting for manual trigger",
				},
			)
			return false, nil
		}
		rs.Status.Conditions.SetCondition(
			status.Condition{
				Type:    volsyncv1alpha1.ConditionSynchronizing,
				Status:  corev1.ConditionTrue,
				Reason:  volsyncv1alpha1.SynchronizingReasonSync,
				Message: "Synchronization in-progress",
			},
		)
		return true, nil
	}

	// If there's no schedule, (and no manual trigger), we should sync
	if rs.Status.NextSyncTime.IsZero() {
		// Condition update omitted intentionally to work with Mover inteface.
		return true, nil
	}

	// if it's past the nextSyncTime, we should sync
	if rs.Status.NextSyncTime.Time.Before(time.Now()) {
		rs.Status.Conditions.SetCondition(
			status.Condition{
				Type:    volsyncv1alpha1.ConditionSynchronizing,
				Status:  corev1.ConditionTrue,
				Reason:  volsyncv1alpha1.SynchronizingReasonSync,
				Message: "Synchronization in-progress",
			},
		)
		return true, nil
	}
	rs.Status.Conditions.SetCondition(
		status.Condition{
			Type:    volsyncv1alpha1.ConditionSynchronizing,
			Status:  corev1.ConditionFalse,
			Reason:  volsyncv1alpha1.SynchronizingReasonSched,
			Message: "Waiting for next scheduled synchronization",
		},
	)
	return false, nil
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

	// When a sync is completed we set lastManualSync
	if rs.Spec.Trigger != nil {
		rs.Status.LastManualSync = rs.Spec.Trigger.Manual
	} else {
		rs.Status.LastManualSync = ""
	}

	return updateNextSyncSource(rs, metrics, logger)
}

type rsyncSrcReconciler struct {
	sourceVolumeHandler
	volsyncMetrics
	service        *corev1.Service
	destSecret     *corev1.Secret
	srcSecret      *corev1.Secret
	serviceAccount *corev1.ServiceAccount
	job            *batchv1.Job
}

type rcloneSrcReconciler struct {
	sourceVolumeHandler
	volsyncMetrics
	rcloneConfigSecret *corev1.Secret
	serviceAccount     *corev1.ServiceAccount
	job                *batchv1.Job
}

//nolint:dupl
func RunRsyncSrcReconciler(
	ctx context.Context,
	instance *volsyncv1alpha1.ReplicationSource,
	sr *ReplicationSourceReconciler,
	logger logr.Logger,
) (ctrl.Result, error) {
	r := rsyncSrcReconciler{
		sourceVolumeHandler: sourceVolumeHandler{
			Ctx:                         ctx,
			Instance:                    instance,
			ReplicationSourceReconciler: *sr,
			Options:                     &instance.Spec.Rsync.ReplicationSourceVolumeOptions,
		},
		volsyncMetrics: newVolSyncMetrics(prometheus.Labels{
			"obj_name":      instance.Name,
			"obj_namespace": instance.Namespace,
			"role":          "source",
			"method":        "rsync",
		}),
	}

	l := logger.WithValues("method", "Rsync")

	// Make sure there's a place to write status info
	if r.Instance.Status.Rsync == nil {
		r.Instance.Status.Rsync = &volsyncv1alpha1.ReplicationSourceRsyncStatus{}
	}

	// wrap the scheduling functions as reconcileFuncs
	awaitNextSync := func(l logr.Logger) (bool, error) {
		return awaitNextSyncSource(r.Instance, r.volsyncMetrics, l)
	}

	_, err := utils.ReconcileBatch(l,
		awaitNextSync,
		r.EnsurePVC,
		r.ensureService,
		r.publishSvcAddress,
		r.ensureKeys,
		r.ensureServiceAccount,
		r.ensureJob,
		r.cleanupJob,
		r.CleanupPVC,
	)
	return ctrl.Result{}, err
}

// RunRcloneSrcReconciler is invoked when ReplicationSource.Spec.Rclone != nil
func RunRcloneSrcReconciler(
	ctx context.Context,
	instance *volsyncv1alpha1.ReplicationSource,
	sr *ReplicationSourceReconciler,
	logger logr.Logger,
) (ctrl.Result, error) {
	r := rcloneSrcReconciler{
		sourceVolumeHandler: sourceVolumeHandler{
			Ctx:                         ctx,
			Instance:                    instance,
			ReplicationSourceReconciler: *sr,
			Options:                     &instance.Spec.Rclone.ReplicationSourceVolumeOptions,
		},
		volsyncMetrics: newVolSyncMetrics(prometheus.Labels{
			"obj_name":      instance.Name,
			"obj_namespace": instance.Namespace,
			"role":          "source",
			"method":        "rclone",
		}),
	}

	l := logger.WithValues("method", "Rclone")

	// wrap the scheduling functions as reconcileFuncs
	awaitNextSync := func(l logr.Logger) (bool, error) {
		return awaitNextSyncSource(r.Instance, r.volsyncMetrics, l)
	}

	_, err := utils.ReconcileBatch(l,
		awaitNextSync,
		r.validateRcloneSpec,
		r.EnsurePVC,
		r.ensureServiceAccount,
		r.ensureRcloneConfig,
		r.ensureJob,
		r.cleanupJob,
		r.CleanupPVC,
	)
	return ctrl.Result{}, err
}

//nolint:funlen
func (r *rcloneSrcReconciler) ensureJob(l logr.Logger) (bool, error) {
	r.job = &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "volsync-rclone-src-" + r.Instance.Name,
			Namespace: r.Instance.Namespace,
		},
	}
	logger := l.WithValues("job", utils.NameFor(r.job))
	op, err := ctrlutil.CreateOrUpdate(r.Ctx, r.Client, r.job, func() error {
		if err := ctrl.SetControllerReference(r.Instance, r.job, r.Scheme); err != nil {
			logger.Error(err, "unable to set controller reference")
			return err
		}
		r.job.Spec.Template.ObjectMeta.Name = r.job.Name
		if r.job.Spec.Template.ObjectMeta.Labels == nil {
			r.job.Spec.Template.ObjectMeta.Labels = map[string]string{}
		}
		backoffLimit := int32(2)
		r.job.Spec.BackoffLimit = &backoffLimit
		if r.Instance.Spec.Paused {
			parallelism := int32(0)
			r.job.Spec.Parallelism = &parallelism
		} else {
			parallelism := int32(1)
			r.job.Spec.Parallelism = &parallelism
		}
		if len(r.job.Spec.Template.Spec.Containers) != 1 {
			r.job.Spec.Template.Spec.Containers = []corev1.Container{{}}
		}

		r.job.Spec.Template.Spec.Containers[0].Name = "rclone"
		r.job.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{
			{Name: "RCLONE_CONFIG", Value: "/rclone-config/rclone.conf"},
			{Name: "RCLONE_DEST_PATH", Value: *r.Instance.Spec.Rclone.RcloneDestPath},
			{Name: "DIRECTION", Value: "source"},
			{Name: "MOUNT_PATH", Value: mountPath},
			{Name: "RCLONE_CONFIG_SECTION", Value: *r.Instance.Spec.Rclone.RcloneConfigSection},
		}
		r.job.Spec.Template.Spec.Containers[0].Command = []string{"/bin/bash", "-c", "./active.sh"}
		r.job.Spec.Template.Spec.Containers[0].Image = RcloneContainerImage
		runAsUser := int64(0)
		r.job.Spec.Template.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{
			RunAsUser: &runAsUser,
		}
		r.job.Spec.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
			{Name: dataVolumeName, MountPath: mountPath},
			{Name: rcloneSecret, MountPath: "/rclone-config/"},
		}
		r.job.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyNever
		r.job.Spec.Template.Spec.ServiceAccountName = r.serviceAccount.Name
		secretMode := int32(0600)
		r.job.Spec.Template.Spec.Volumes = []corev1.Volume{
			{Name: dataVolumeName, VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: r.PVC.Name,
				}},
			},
			{Name: rcloneSecret, VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  r.rcloneConfigSecret.Name,
					DefaultMode: &secretMode,
				}},
			},
		}
		logger.V(1).Info("Job has PVC", "PVC", r.PVC, "DS", r.PVC.Spec.DataSource)
		return nil
	})
	// If Job had failed, delete it so it can be recreated
	if r.job.Status.Failed >= *r.job.Spec.BackoffLimit {
		logger.Info("deleting job -- backoff limit reached")
		err = r.Client.Delete(r.Ctx, r.job, client.PropagationPolicy(metav1.DeletePropagationBackground))
		return false, err
	}
	if err != nil {
		logger.Error(err, "reconcile failed")
	} else {
		logger.V(1).Info("Job reconciled", "operation", op)
	}
	// We only continue reconciling if the rclone job has completed
	return r.job.Status.Succeeded == 1, nil
}

func (r *rsyncSrcReconciler) serviceSelector() map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":      "src-" + r.Instance.Name,
		"app.kubernetes.io/component": "rsync-mover",
		"app.kubernetes.io/part-of":   "volsync",
	}
}

// ensureService maintains the Service that is used to connect to the
// source rsync mover.
func (r *rsyncSrcReconciler) ensureService(l logr.Logger) (bool, error) {
	if r.Instance.Spec.Rsync.Address != nil {
		// Connection will be outbound. Don't need a Service
		return true, nil
	}

	r.service = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "volsync-rsync-src-" + r.Instance.Name,
			Namespace: r.Instance.Namespace,
		},
	}
	svcDesc := rsyncSvcDescription{
		Context:  r.Ctx,
		Client:   r.Client,
		Scheme:   r.Scheme,
		Service:  r.service,
		Owner:    r.Instance,
		Type:     r.Instance.Spec.Rsync.ServiceType,
		Selector: r.serviceSelector(),
		Port:     r.Instance.Spec.Rsync.Port,
	}
	return svcDesc.Reconcile(l)
}

func (r *rsyncSrcReconciler) publishSvcAddress(l logr.Logger) (bool, error) {
	if r.service == nil { // no service, nothing to do
		r.Instance.Status.Rsync.Address = nil
		return true, nil
	}

	address := getServiceAddress(r.service)
	if address == "" {
		// We don't have an address yet, try again later
		r.Instance.Status.Rsync.Address = nil
		return false, nil
	}
	r.Instance.Status.Rsync.Address = &address

	l.V(1).Info("Service addr published", "address", address)
	return true, nil
}

//nolint:dupl
func (r *rsyncSrcReconciler) ensureKeys(l logr.Logger) (bool, error) {
	// If user provided keys, use those
	if r.Instance.Spec.Rsync.SSHKeys != nil {
		r.srcSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      *r.Instance.Spec.Rsync.SSHKeys,
				Namespace: r.Instance.Namespace,
			},
		}
		fields := []string{"source", "source.pub", "destination.pub"}
		if err := getAndValidateSecret(r.Ctx, r.Client, l, r.srcSecret, fields); err != nil {
			l.Error(err, "SSH keys secret does not contain the proper fields")
			return false, err
		}
		return true, nil
	}

	// otherwise, we need to create our own
	keyInfo := rsyncSSHKeys{
		Context:      r.Ctx,
		Client:       r.Client,
		Scheme:       r.Scheme,
		Owner:        r.Instance,
		NameTemplate: "volsync-rsync-src",
	}
	cont, err := keyInfo.Reconcile(l)
	if !cont || err != nil {
		r.Instance.Status.Rsync.SSHKeys = nil
	} else {
		r.srcSecret = keyInfo.SrcSecret
		r.destSecret = keyInfo.DestSecret
		r.Instance.Status.Rsync.SSHKeys = &r.destSecret.Name
	}
	return cont, err
}

func (r *rcloneSrcReconciler) ensureRcloneConfig(l logr.Logger) (bool, error) {
	// If user provided "rclone-secret", use those

	r.rcloneConfigSecret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      *r.Instance.Spec.Rclone.RcloneConfig,
			Namespace: r.Instance.Namespace,
		},
	}
	fields := []string{"rclone.conf"}
	if err := getAndValidateSecret(r.Ctx, r.Client, l, r.rcloneConfigSecret, fields); err != nil {
		l.Error(err, "Rclone config secret does not contain the proper fields")
		return false, err
	}
	return true, nil
}

func (r *rsyncSrcReconciler) ensureServiceAccount(l logr.Logger) (bool, error) {
	r.serviceAccount = &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "volsync-rsync-src-" + r.Instance.Name,
			Namespace: r.Instance.Namespace,
		},
	}
	saDesc := utils.NewSAHandler(r.Ctx, r.Client, r.Instance, r.serviceAccount)
	return saDesc.Reconcile(l)
}

func (r *rcloneSrcReconciler) ensureServiceAccount(l logr.Logger) (bool, error) {
	r.serviceAccount = &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "volsync-src-" + r.Instance.Name,
			Namespace: r.Instance.Namespace,
		},
	}
	saDesc := utils.NewSAHandler(r.Ctx, r.Client, r.Instance, r.serviceAccount)
	return saDesc.Reconcile(l)
}

//nolint:funlen
func (r *rsyncSrcReconciler) ensureJob(l logr.Logger) (bool, error) {
	r.job = &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "volsync-rsync-src-" + r.Instance.Name,
			Namespace: r.Instance.Namespace,
		},
	}
	logger := l.WithValues("job", utils.NameFor(r.job))

	op, err := ctrlutil.CreateOrUpdate(r.Ctx, r.Client, r.job, func() error {
		if err := ctrl.SetControllerReference(r.Instance, r.job, r.Scheme); err != nil {
			logger.Error(err, "unable to set controller reference")
			return err
		}
		r.job.Spec.Template.ObjectMeta.Name = r.job.Name
		if r.job.Spec.Template.ObjectMeta.Labels == nil {
			r.job.Spec.Template.ObjectMeta.Labels = map[string]string{}
		}
		for k, v := range r.serviceSelector() {
			r.job.Spec.Template.ObjectMeta.Labels[k] = v
		}
		backoffLimit := int32(2)
		r.job.Spec.BackoffLimit = &backoffLimit
		if r.Instance.Spec.Paused {
			parallelism := int32(0)
			r.job.Spec.Parallelism = &parallelism
		} else {
			parallelism := int32(1)
			r.job.Spec.Parallelism = &parallelism
		}
		if len(r.job.Spec.Template.Spec.Containers) != 1 {
			r.job.Spec.Template.Spec.Containers = []corev1.Container{{}}
		}
		r.job.Spec.Template.Spec.Containers[0].Name = "rsync"
		if r.Instance.Spec.Rsync.Port != nil && r.Instance.Spec.Rsync.Address != nil {
			connectPort := strconv.Itoa(int(*r.Instance.Spec.Rsync.Port))
			r.job.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{
				{Name: "DESTINATION_ADDRESS", Value: *r.Instance.Spec.Rsync.Address},
				{Name: "DESTINATION_PORT", Value: connectPort},
			}
		} else if r.Instance.Spec.Rsync.Port == nil && r.Instance.Spec.Rsync.Address != nil {
			r.job.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{
				{Name: "DESTINATION_ADDRESS", Value: *r.Instance.Spec.Rsync.Address},
			}
		} else if r.Instance.Spec.Rsync.Address == nil {
			r.job.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{}
		}
		r.job.Spec.Template.Spec.Containers[0].Command = []string{"/bin/bash", "-c", "/source.sh"}
		r.job.Spec.Template.Spec.Containers[0].Image = RsyncContainerImage
		runAsUser := int64(0)
		r.job.Spec.Template.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{
			Capabilities: &corev1.Capabilities{
				Add: []corev1.Capability{
					"AUDIT_WRITE",
					"SYS_CHROOT",
				},
			},
			RunAsUser: &runAsUser,
		}
		r.job.Spec.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
			{Name: dataVolumeName, MountPath: mountPath},
			{Name: "keys", MountPath: "/keys"},
		}
		r.job.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyNever
		r.job.Spec.Template.Spec.ServiceAccountName = r.serviceAccount.Name
		secretMode := int32(0600)
		r.job.Spec.Template.Spec.Volumes = []corev1.Volume{
			{Name: dataVolumeName, VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: r.PVC.Name,
				}},
			},
			{Name: "keys", VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  r.srcSecret.Name,
					DefaultMode: &secretMode,
				}},
			},
		}
		logger.V(1).Info("Job has PVC", "PVC", r.PVC, "DS", r.PVC.Spec.DataSource)
		return nil
	})

	// If Job had failed, delete it so it can be recreated
	if r.job.Status.Failed >= *r.job.Spec.BackoffLimit {
		logger.Info("deleting job -- backoff limit reached")
		err = r.Client.Delete(r.Ctx, r.job, client.PropagationPolicy(metav1.DeletePropagationBackground))
		return false, err
	}

	if err != nil {
		logger.Error(err, "reconcile failed")
	} else {
		logger.V(1).Info("Job reconciled", "operation", op)
	}

	// We only continue reconciling if the rsync job has completed
	return r.job.Status.Succeeded == 1, nil
}

//nolint:dupl
func (r *rsyncSrcReconciler) cleanupJob(l logr.Logger) (bool, error) {
	logger := l.WithValues("job", r.job)
	// update time/duration
	if cont, err := updateLastSyncSource(r.Instance, r.volsyncMetrics, logger); !cont || err != nil {
		return cont, err
	}
	if r.job.Status.StartTime != nil {
		d := r.Instance.Status.LastSyncTime.Sub(r.job.Status.StartTime.Time)
		r.Instance.Status.LastSyncDuration = &metav1.Duration{Duration: d}
		r.SyncDurations.Observe(d.Seconds())
	}
	// remove job
	if err := r.Client.Delete(r.Ctx, r.job, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
		logger.Error(err, "unable to delete job")
		return false, err
	}
	return true, nil
}

//nolint:dupl
func (r *rcloneSrcReconciler) cleanupJob(l logr.Logger) (bool, error) {
	logger := l.WithValues("job", r.job)
	// update time/duration
	if cont, err := updateLastSyncSource(r.Instance, r.volsyncMetrics, logger); !cont || err != nil {
		return cont, err
	}
	if r.job.Status.StartTime != nil {
		d := r.Instance.Status.LastSyncTime.Sub(r.job.Status.StartTime.Time)
		r.Instance.Status.LastSyncDuration = &metav1.Duration{Duration: d}
		r.SyncDurations.Observe(d.Seconds())
	}
	// remove job
	if r.job.Status.Succeeded >= 1 {
		if err := r.Client.Delete(r.Ctx, r.job, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
			logger.Error(err, "unable to delete job")
			return false, err
		}
		logger.Info("Job deleted", "Job name: ", r.job.Spec.Template.ObjectMeta.Name)
	}
	return true, nil
}

func (r *rcloneSrcReconciler) validateRcloneSpec(l logr.Logger) (bool, error) {
	if len(*r.Instance.Spec.Rclone.RcloneConfig) == 0 {
		err := errors.New("Unable to get Rclone config secret name")
		l.V(1).Info("Unable to get Rclone config secret name")
		return false, err
	}
	if len(*r.Instance.Spec.Rclone.RcloneConfigSection) == 0 {
		err := errors.New("Unable to get Rclone config section name")
		l.V(1).Info("Unable to get Rclone config section name")

		return false, err
	}
	if len(*r.Instance.Spec.Rclone.RcloneDestPath) == 0 {
		err := errors.New("Unable to get Rclone destination name")
		l.V(1).Info("Unable to get Rclone destination name")

		return false, err
	}
	return true, nil
}
