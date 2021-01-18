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
	"errors"
	"time"

	"github.com/go-logr/logr"
	snapv1 "github.com/kubernetes-csi/external-snapshotter/v2/pkg/apis/volumesnapshot/v1beta1"
	"github.com/operator-framework/operator-lib/status"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	scribev1alpha1 "github.com/backube/scribe/api/v1alpha1"
)

const (
	// DefaultRsyncContainerImage is the default container image name of the rsync data mover
	DefaultRsyncContainerImage = "quay.io/backube/scribe-mover-rsync:latest"
)

// RsyncContainerImage is the container image name of the rsync data mover
var RsyncContainerImage string

// RcloneContainerImage is the container image name of the rclone data mover
var RcloneContainerImage string = "quay.io/backube/scribe-mover-rclone:latest"

// ReplicationDestinationReconciler reconciles a ReplicationDestination object
type ReplicationDestinationReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

//nolint:lll
//+kubebuilder:rbac:groups=scribe.backube,resources=replicationdestinations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=scribe.backube,resources=replicationdestinations/finalizers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=scribe.backube,resources=replicationdestinations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,resourceNames=scribe-mover,verbs=use
//+kubebuilder:rbac:groups=snapshot.storage.k8s.io,resources=volumesnapshots,verbs=get;list;watch;create;update;patch;delete

func (r *ReplicationDestinationReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	logger := r.Log.WithValues("replicationdestination", req.NamespacedName)

	// Get CR instance
	inst := &scribev1alpha1.ReplicationDestination{}
	if err := r.Client.Get(ctx, req.NamespacedName, inst); err != nil {
		if !kerrors.IsNotFound(err) {
			logger.Error(err, "Failed to get Destination")
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Prepare the .Status fields if necessary
	if inst.Status == nil {
		inst.Status = &scribev1alpha1.ReplicationDestinationStatus{}
	}
	if inst.Status.Conditions == nil {
		inst.Status.Conditions = status.Conditions{}
	}

	var result ctrl.Result
	var err error
	// Only reconcile if the replication method is internal
	if inst.Spec.Rsync != nil {
		result, err = RunRsyncDestReconciler(ctx, inst, r, logger)
	} else if inst.Spec.Rclone != nil {
		result, err = RunRcloneDestReconciler(ctx, inst, r, logger)
	} else {
		// Not an internal method... we're done.
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

func (r *ReplicationDestinationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&scribev1alpha1.ReplicationDestination{}).
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

type rsyncDestReconciler struct {
	destinationVolumeHandler
	service        *corev1.Service
	destSecret     *corev1.Secret
	srcSecret      *corev1.Secret
	serviceAccount *corev1.ServiceAccount
	job            *batchv1.Job
}

type rcloneDestReconciler struct {
	destinationVolumeHandler
	rcloneConfigSecret *corev1.Secret
	serviceAccount     *corev1.ServiceAccount
	job                *batchv1.Job
}

func RunRsyncDestReconciler(ctx context.Context, instance *scribev1alpha1.ReplicationDestination,
	dr *ReplicationDestinationReconciler, logger logr.Logger) (ctrl.Result, error) {
	// Initialize state for the reconcile pass
	r := rsyncDestReconciler{
		destinationVolumeHandler: destinationVolumeHandler{
			Ctx:                              ctx,
			Instance:                         instance,
			ReplicationDestinationReconciler: *dr,
			Options:                          &instance.Spec.Rsync.ReplicationDestinationVolumeOptions,
		},
	}

	l := logger.WithValues("method", "Rsync")

	// Make sure there's a place to write status info
	if r.Instance.Status.Rsync == nil {
		r.Instance.Status.Rsync = &scribev1alpha1.ReplicationDestinationRsyncStatus{}
	}

	_, err := reconcileBatch(l,
		r.EnsurePVC,
		r.ensureService,
		r.publishSvcAddress,
		r.ensureSecrets,
		r.ensureServiceAccount,
		r.ensureJob,
		r.PreserveImage,
		r.cleanupJob,
	)
	return ctrl.Result{}, err
}

// RunRcloneDestReconciler reconciles rclone mover related objects.
func RunRcloneDestReconciler(ctx context.Context, instance *scribev1alpha1.ReplicationDestination,
	dr *ReplicationDestinationReconciler, logger logr.Logger) (ctrl.Result, error) {
	// Initialize state for the reconcile pass
	r := rcloneDestReconciler{
		destinationVolumeHandler: destinationVolumeHandler{
			Ctx:                              ctx,
			Instance:                         instance,
			ReplicationDestinationReconciler: *dr,
			Options:                          &instance.Spec.Rclone.ReplicationDestinationVolumeOptions,
		},
	}
	l := logger.WithValues("method", "Rclone")
	_, err := reconcileBatch(l,
		r.validateRcloneSpec,
		r.EnsurePVC,
		r.ensureRcloneConfig,
		r.ensureServiceAccount,
		r.ensureJob,
		r.PreserveImage,
		r.cleanupJob,
	)
	return ctrl.Result{}, err
}
func (r *rsyncDestReconciler) serviceSelector() map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":      "dest-" + r.Instance.Name,
		"app.kubernetes.io/component": "rsync-mover",
		"app.kubernetes.io/part-of":   "scribe",
	}
}

// ensureService maintains the Service that is used to connect to the
// destination rsync mover.
func (r *rsyncDestReconciler) ensureService(l logr.Logger) (bool, error) {
	if r.Instance.Spec.Rsync.Address != nil {
		// Connection will be outbound. Don't need a Service
		return true, nil
	}

	r.service = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "scribe-rsync-dest-" + r.Instance.Name,
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

func (r *rsyncDestReconciler) publishSvcAddress(l logr.Logger) (bool, error) {
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
func (r *rsyncDestReconciler) ensureSecrets(l logr.Logger) (bool, error) {
	// If user provided keys, use those
	if r.Instance.Spec.Rsync.SSHKeys != nil {
		r.destSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      *r.Instance.Spec.Rsync.SSHKeys,
				Namespace: r.Instance.Namespace,
			},
		}
		fields := []string{"destination", "destination.pub", "source.pub"}
		if err := getAndValidateSecret(r.Ctx, r.Client, l, r.destSecret, fields); err != nil {
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
		NameTemplate: "scribe-rsync-dest",
	}
	cont, err := keyInfo.Reconcile(l)
	if !cont || err != nil {
		r.Instance.Status.Rsync.SSHKeys = nil
	} else {
		r.srcSecret = keyInfo.SrcSecret
		r.destSecret = keyInfo.DestSecret
		r.Instance.Status.Rsync.SSHKeys = &r.srcSecret.Name
	}
	return cont, err
}

func (r *rcloneDestReconciler) ensureRcloneConfig(l logr.Logger) (bool, error) {
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

func (r *rsyncDestReconciler) ensureServiceAccount(l logr.Logger) (bool, error) {
	r.serviceAccount = &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "scribe-rsync-dest-" + r.Instance.Name,
			Namespace: r.Instance.Namespace,
		},
	}
	saDesc := rsyncSADescription{
		Context: r.Ctx,
		Client:  r.Client,
		Scheme:  r.Scheme,
		SA:      r.serviceAccount,
		Owner:   r.Instance,
	}
	return saDesc.Reconcile(l)
}

func (r *rcloneDestReconciler) ensureServiceAccount(l logr.Logger) (bool, error) {
	r.serviceAccount = &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "scribe-src-" + r.Instance.Name,
			Namespace: r.Instance.Namespace,
		},
	}
	saDesc := rsyncSADescription{
		Context: r.Ctx,
		Client:  r.Client,
		Scheme:  r.Scheme,
		SA:      r.serviceAccount,
		Owner:   r.Instance,
	}
	return saDesc.Reconcile(l)
}

//nolint:funlen
func (r *rsyncDestReconciler) ensureJob(l logr.Logger) (bool, error) {
	jobName := types.NamespacedName{
		Name:      "scribe-rsync-dest-" + r.Instance.Name,
		Namespace: r.Instance.Namespace,
	}
	logger := l.WithValues("job", jobName)

	r.job = &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName.Name,
			Namespace: jobName.Namespace,
		},
	}

	op, err := ctrlutil.CreateOrUpdate(r.Ctx, r.Client, r.job, func() error {
		if err := ctrl.SetControllerReference(r.Instance, r.job, r.Scheme); err != nil {
			logger.Error(err, "unable to set controller reference")
			return err
		}
		r.job.Spec.Template.ObjectMeta.Name = jobName.Name
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
		r.job.Spec.Template.Spec.Containers[0].Command = []string{"/bin/bash", "-c", "/destination.sh"}
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
			{Name: dataVolumeName, MountPath: "/data"},
			{Name: "keys", MountPath: "/keys"},
		}
		r.job.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyNever
		r.job.Spec.Template.Spec.ServiceAccountName = r.serviceAccount.Name
		secretMode := int32(0600)
		r.job.Spec.Template.Spec.Volumes = []corev1.Volume{
			{Name: dataVolumeName, VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: r.PVC.Name,
					ReadOnly:  false,
				}},
			},
			{Name: "keys", VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  r.destSecret.Name,
					DefaultMode: &secretMode,
				}},
			},
		}
		return nil
	})

	// If Job had failed, delete it so it can be recreated
	if r.job.Status.Failed == *r.job.Spec.BackoffLimit {
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

//nolint:funlen
func (r *rcloneDestReconciler) ensureJob(l logr.Logger) (bool, error) {
	r.job = &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "scribe-rclone-src-" + r.Instance.Name,
			Namespace: r.Instance.Namespace,
		},
	}
	logger := l.WithValues("job", nameFor(r.job))
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
		if len(r.job.Spec.Template.Spec.Containers) != 1 {
			r.job.Spec.Template.Spec.Containers = []corev1.Container{{}}
		}

		r.job.Spec.Template.Spec.Containers[0].Name = "rclone"
		r.job.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{
			{Name: "RCLONE_CONFIG", Value: "/rclone-config/rclone.conf"},
			{Name: "RCLONE_DEST_PATH", Value: *r.Instance.Spec.Rclone.RcloneDestPath},
			{Name: "DIRECTION", Value: "destination"},
			{Name: "MOUNT_PATH", Value: MOUNTPATH},
			{Name: "RCLONE_CONFIG_SECTION", Value: *r.Instance.Spec.Rclone.RcloneConfigSection},
		}
		r.job.Spec.Template.Spec.Containers[0].Command = []string{"/bin/bash", "-c", "./active.sh"}
		r.job.Spec.Template.Spec.Containers[0].Image = RcloneContainerImage
		runAsUser := int64(0)
		r.job.Spec.Template.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{
			RunAsUser: &runAsUser,
		}
		r.job.Spec.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
			{Name: dataVolumeName, MountPath: "/data"},
			{Name: *r.Instance.Spec.Rclone.RcloneConfig, MountPath: "/rclone-config/"},
		}
		r.job.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyNever
		r.job.Spec.Template.Spec.ServiceAccountName = r.serviceAccount.Name
		secretMode := int32(0600)
		r.job.Spec.Template.Spec.Volumes = []corev1.Volume{
			{Name: dataVolumeName, VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: r.PVC.Name,
					ReadOnly:  false,
				}},
			},
			{Name: *r.Instance.Spec.Rclone.RcloneConfig, VolumeSource: corev1.VolumeSource{
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
	if r.job.Status.Failed == *r.job.Spec.BackoffLimit {
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

func (r *rsyncDestReconciler) cleanupJob(l logr.Logger) (bool, error) {
	logger := l.WithValues("job", r.job)
	r.Instance.Status.LastSyncTime = &metav1.Time{Time: time.Now()}
	if r.job.Status.StartTime != nil {
		d := r.Instance.Status.LastSyncTime.Sub(r.job.Status.StartTime.Time)
		r.Instance.Status.LastSyncDuration = &metav1.Duration{Duration: d}
	}

	// Delete the (completed) Job. The next reconcile pass will recreate it.
	// Set propagation policy so the old pods get deleted
	if err := r.Client.Delete(r.Ctx, r.job, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
		logger.Error(err, "unable to delete old job")
		return false, err
	}

	return true, nil
}

func (r *rcloneDestReconciler) cleanupJob(l logr.Logger) (bool, error) {
	logger := l.WithValues("job", r.job)
	// update time/duration

	r.Instance.Status.LastSyncTime = &metav1.Time{Time: time.Now()}
	if r.job.Status.StartTime != nil {
		d := r.Instance.Status.LastSyncTime.Sub(r.job.Status.StartTime.Time)
		r.Instance.Status.LastSyncDuration = &metav1.Duration{Duration: d}
	}
	// remove job
	if r.job.Status.Succeeded >= 1 {
		logger.Info("Job succeeded", "Job", r.job.Spec)

		if err := r.Client.Delete(r.Ctx, r.job, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
			logger.Error(err, "unable to delete job")
			return false, err
		}
		logger.Info("Job deleted", "Job", r.job.Spec)
	}
	return true, nil
}

func (r *rcloneDestReconciler) validateRcloneSpec(l logr.Logger) (bool, error) {
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
	l.V(1).Info("Rclone config validation complete.")
	return true, nil
}
