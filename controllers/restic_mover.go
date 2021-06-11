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
	"fmt"
	"time"

	scribev1alpha1 "github.com/backube/scribe/api/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	// DefaultResticContainerImage is the default container image for the restic
	// data mover
	DefaultResticContainerImage = "quay.io/backube/scribe-mover-restic:latest"
	resticCacheMountPath        = "/cache"
)

var (
	// ResticContainerImage is the container image name of the restic data mover
	ResticContainerImage string
)

type resticSrcReconciler struct {
	sourceVolumeHandler
	scribeMetrics
	resticRepositorySecret *corev1.Secret
	serviceAccount         *corev1.ServiceAccount
	job                    *batchv1.Job
}

type resticDestReconciler struct {
	destinationVolumeHandler
	scribeMetrics
	resticRepositorySecret *corev1.Secret
	serviceAccount         *corev1.ServiceAccount
	job                    *batchv1.Job
}

// RunResticSrcReconciler is invoked when ReplicationSource.Spec>Restic !=  nil
//nolint:dupl
func RunResticSrcReconciler(
	ctx context.Context,
	instance *scribev1alpha1.ReplicationSource,
	sr *ReplicationSourceReconciler,
	logger logr.Logger,
) (ctrl.Result, error) {
	r := resticSrcReconciler{
		sourceVolumeHandler: sourceVolumeHandler{
			Ctx:                         ctx,
			Instance:                    instance,
			ReplicationSourceReconciler: *sr,
			Options:                     &instance.Spec.Restic.ReplicationSourceVolumeOptions,
		},
		scribeMetrics: newScribeMetrics(prometheus.Labels{
			"obj_name":      instance.Name,
			"obj_namespace": instance.Namespace,
			"role":          "source",
			"method":        "restic",
		}),
	}
	l := logger.WithValues("method", "Restic")
	// Create ReplicationSourceResticStatus to write restic status
	if r.Instance.Status.Restic == nil {
		r.Instance.Status.Restic = &scribev1alpha1.ReplicationSourceResticStatus{}
	}

	//Wrap the scheduling functions as reconcileFuncs
	awaitNextSync := func(l logr.Logger) (bool, error) {
		return awaitNextSyncSource(r.Instance, r.scribeMetrics, l)
	}

	_, err := reconcileBatch(l,
		awaitNextSync,
		r.validateResticSpec,
		r.EnsurePVC,
		r.pvcForCache,
		r.ensureServiceAccount,
		r.ensureRepository,
		r.ensureJob,
		r.cleanupJob,
		r.CleanupPVC,
	)
	return ctrl.Result{}, err
}

//nolint:dupl,funlen
func (r *resticSrcReconciler) ensureJob(l logr.Logger) (bool, error) {
	r.job = &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "scribe-restic-src-" + r.Instance.Name,
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
		r.job.Spec.Template.Spec.Containers[0].Name = "restic-backup"

		var optionalFalse = false
		forgetOptions := generateForgetOptions(r.Instance.Spec.Restic.Retain)
		l.V(1).Info("restic forget options", "options", forgetOptions)
		r.job.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{
			{Name: "FORGET_OPTIONS", Value: forgetOptions},
			{Name: "DATA_DIR", Value: mountPath},
			{Name: "RESTIC_CACHE_DIR", Value: resticCacheMountPath},
			{Name: "RESTIC_REPOSITORY", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: r.resticRepositorySecret.Name,
					},
					Key:      "RESTIC_REPOSITORY",
					Optional: &optionalFalse,
				},
			}},
			{Name: "RESTIC_PASSWORD", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: r.resticRepositorySecret.Name,
					},
					Key:      "RESTIC_PASSWORD",
					Optional: &optionalFalse,
				},
			}},
			{Name: "AWS_ACCESS_KEY_ID", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: r.resticRepositorySecret.Name,
					},
					Key:      "AWS_ACCESS_KEY_ID",
					Optional: &optionalFalse,
				},
			}},
			{Name: "AWS_SECRET_ACCESS_KEY", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: r.resticRepositorySecret.Name,
					},
					Key:      "AWS_SECRET_ACCESS_KEY",
					Optional: &optionalFalse,
				},
			}},
		}

		r.job.Spec.Template.Spec.Containers[0].Command = []string{"/entry.sh"}

		if r.resticPrune(l) {
			r.job.Spec.Template.Spec.Containers[0].Args = []string{"backup", "prune"}
		} else {
			r.job.Spec.Template.Spec.Containers[0].Args = []string{"backup"}
		}
		r.job.Spec.Template.Spec.Containers[0].Image = ResticContainerImage
		runAsUser := int64(0)
		r.job.Spec.Template.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{
			RunAsUser: &runAsUser,
		}
		r.job.Spec.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
			{Name: dataVolumeName, MountPath: mountPath},
			{Name: resticCache, MountPath: resticCacheMountPath},
		}
		r.job.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyNever
		r.job.Spec.Template.Spec.ServiceAccountName = r.serviceAccount.Name
		r.job.Spec.Template.Spec.Volumes = []corev1.Volume{
			{Name: dataVolumeName, VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: r.PVC.Name,
				}},
			},
			{Name: resticCache, VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: r.resticCache.Name,
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
	// Only set r.Instance.Status.Restic.LastPruned when the restic job has completed
	if r.resticPrune(l) && r.job.Status.Succeeded == 1 {
		r.Instance.Status.Restic.LastPruned = &metav1.Time{Time: time.Now()}
		l.V(1).Info("Prune completed at ", ".Status.Restic.LastPruned", r.Instance.Status.Restic.LastPruned)
	}
	// We only continue reconciling if the restic job has completed
	return r.job.Status.Succeeded == 1, nil
}

//nolint:funlen
func (r *resticSrcReconciler) resticPrune(l logr.Logger) bool {
	pruneInterval := int64(*r.Instance.Spec.Restic.PruneIntervalDays * 24)
	pruneIntervalHours := time.Duration(pruneInterval) * time.Hour
	creationTime := r.Instance.ObjectMeta.CreationTimestamp
	now := time.Now()
	shouldPrune := false
	var delta time.Time

	if r.Instance.Status.Restic.LastPruned == nil {
		//This is the first prune and never has been pruned before
		//Check if now - CreationTime > pruneInterval
		// true: start first prune and update LastPruned in Status.Restic
		// false: wait for next prune
		delta = creationTime.Time.Add(pruneIntervalHours)
		shouldPrune = now.After(delta)
	}
	if r.Instance.Status.Restic.LastPruned != nil {
		//calculate next prune time as now - lastPruned > pruneInterval
		delta = r.Instance.Status.Restic.LastPruned.Time.Add(pruneIntervalHours)
		shouldPrune = now.After(delta)
	}
	if !shouldPrune {
		l.V(1).Info("Skipping prune", "next", delta)
	}
	return shouldPrune
}

func (r *resticSrcReconciler) ensureRepository(l logr.Logger) (bool, error) {
	// If user provided "repository-secret", use those

	r.resticRepositorySecret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Instance.Spec.Restic.Repository,
			Namespace: r.Instance.Namespace,
		},
	}
	fields := []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "RESTIC_PASSWORD", "RESTIC_REPOSITORY"}
	if err := getAndValidateSecret(r.Ctx, r.Client, l, r.resticRepositorySecret, fields); err != nil {
		l.Error(err, "Restic config secret does not contain the proper fields")
		return false, err
	}
	return true, nil
}

func (r *resticSrcReconciler) ensureServiceAccount(l logr.Logger) (bool, error) {
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

//nolint:dupl
func (r *resticSrcReconciler) cleanupJob(l logr.Logger) (bool, error) {
	logger := l.WithValues("job", r.job)
	// update time/duration
	if _, err := updateLastSyncSource(r.Instance, r.scribeMetrics, logger); err != nil {
		return false, err
	}
	if r.job.Status.StartTime != nil {
		d := r.Instance.Status.LastSyncTime.Sub(r.job.Status.StartTime.Time)
		r.Instance.Status.LastSyncDuration = &metav1.Duration{Duration: d}
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

//nolint:dupl
func (r *resticSrcReconciler) validateResticSpec(l logr.Logger) (bool, error) {
	var err error
	var result bool = true
	if len(r.Instance.Spec.Restic.Repository) == 0 {
		err = errors.New("Unable to get restic repository configurations")
		l.V(1).Info("Unable to get restic repository configurations")
		result = false
	}
	if err == nil {
		// get secret from cluster
		foundSecret := &corev1.Secret{}
		secretNotFoundErr := r.Client.Get(r.Ctx,
			types.NamespacedName{
				Name:      r.Instance.Spec.Restic.Repository,
				Namespace: r.Instance.Namespace,
			}, foundSecret)
		if secretNotFoundErr != nil {
			l.Error(err, "restic repository secret not found.",
				"repository", r.Instance.Spec.Restic.Repository)
			result = false
			err = secretNotFoundErr
		} else {
			r.resticRepositorySecret = foundSecret
		}
	}
	return result, err
}

func generateForgetOptions(policy *scribev1alpha1.ResticRetainPolicy) string {
	const defaultForget = "--keep-last 1"

	if policy == nil { // Retain policy isn't present
		return defaultForget
	}

	var forget string
	optionTable := []struct {
		opt   string
		value *int32
	}{
		{"--keep-hourly", policy.Hourly},
		{"--keep-daily", policy.Daily},
		{"--keep-weekly", policy.Weekly},
		{"--keep-monthly", policy.Monthly},
		{"--keep-yearly", policy.Yearly},
	}
	for _, v := range optionTable {
		if v.value != nil {
			forget += fmt.Sprintf(" %s %d", v.opt, *v.value)
		}
	}
	if policy.Within != nil {
		forget += fmt.Sprintf(" --keep-within %s", *policy.Within)
	}

	if len(forget) == 0 { // Retain policy was present, but empty
		return defaultForget
	}
	return forget
}

// RunResticDestReconciler is invokded when ReplicationDestination.Spec>Restic !=  nil
//nolint:dupl
func RunResticDestReconciler(
	ctx context.Context,
	instance *scribev1alpha1.ReplicationDestination,
	dr *ReplicationDestinationReconciler,
	logger logr.Logger,
) (ctrl.Result, error) {
	r := resticDestReconciler{
		destinationVolumeHandler: destinationVolumeHandler{
			Ctx:                              ctx,
			Instance:                         instance,
			ReplicationDestinationReconciler: *dr,
			Options:                          &instance.Spec.Restic.ReplicationDestinationVolumeOptions,
		},
		scribeMetrics: newScribeMetrics(prometheus.Labels{
			"obj_name":      instance.Name,
			"obj_namespace": instance.Namespace,
			"role":          "destination",
			"method":        "restic",
		}),
	}
	l := logger.WithValues("method", "Restic")

	//Wrap the scheduling functions as reconcileFuncs
	awaitNextSync := func(l logr.Logger) (bool, error) {
		return awaitNextSyncDestination(r.Instance, r.scribeMetrics, l)
	}

	_, err := reconcileBatch(l,
		awaitNextSync,
		r.validateResticSpec,
		r.EnsurePVC,
		r.pvcForCache,
		r.ensureServiceAccount,
		r.ensureRepository,
		r.ensureJob,
		r.PreserveImage,
		r.cleanupJob,
	)
	return ctrl.Result{}, err
}
func (r *resticDestReconciler) ensureServiceAccount(l logr.Logger) (bool, error) {
	r.serviceAccount = &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "scribe-restic-dest-" + r.Instance.Name,
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

func (r *resticDestReconciler) ensureRepository(l logr.Logger) (bool, error) {
	// If user provided "repository-secret", use those

	r.resticRepositorySecret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Instance.Spec.Restic.Repository,
			Namespace: r.Instance.Namespace,
		},
	}
	fields := []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "RESTIC_PASSWORD", "RESTIC_REPOSITORY"}
	if err := getAndValidateSecret(r.Ctx, r.Client, l, r.resticRepositorySecret, fields); err != nil {
		l.Error(err, "Restic config secret does not contain the proper fields")
		return false, err
	}
	return true, nil
}

//nolint:dupl,funlen
func (r *resticDestReconciler) ensureJob(l logr.Logger) (bool, error) {
	r.job = &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "scribe-restic-dest-" + r.Instance.Name,
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
		r.job.Spec.Template.Spec.Containers[0].Name = "restic-restore"
		// calculate retention policy. for now setting FORGET_OPTIONS in
		// env variables directly. It has to be calculated from retention
		// policy
		// get secret from cluster
		var optionalFalse = false
		r.job.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{
			{Name: "FORGET_OPTIONS", Value: "--keep-hourly 2 --keep-daily 1"},
			{Name: "DATA_DIR", Value: mountPath},
			{Name: "RESTIC_CACHE_DIR", Value: resticCacheMountPath},
			{Name: "RESTIC_REPOSITORY", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: r.resticRepositorySecret.Name,
					},
					Key:      "RESTIC_REPOSITORY",
					Optional: &optionalFalse,
				},
			}},
			{Name: "RESTIC_PASSWORD", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: r.resticRepositorySecret.Name,
					},
					Key:      "RESTIC_PASSWORD",
					Optional: &optionalFalse,
				},
			}},
			{Name: "AWS_ACCESS_KEY_ID", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: r.resticRepositorySecret.Name,
					},
					Key:      "AWS_ACCESS_KEY_ID",
					Optional: &optionalFalse,
				},
			}},
			{Name: "AWS_SECRET_ACCESS_KEY", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: r.resticRepositorySecret.Name,
					},
					Key:      "AWS_SECRET_ACCESS_KEY",
					Optional: &optionalFalse,
				},
			}},
		}

		r.job.Spec.Template.Spec.Containers[0].Command = []string{"/entry.sh"}
		r.job.Spec.Template.Spec.Containers[0].Args = []string{"restore"}
		r.job.Spec.Template.Spec.Containers[0].Image = ResticContainerImage
		runAsUser := int64(0)
		r.job.Spec.Template.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{
			RunAsUser: &runAsUser,
		}
		r.job.Spec.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
			{Name: dataVolumeName, MountPath: mountPath},
			{Name: resticCache, MountPath: resticCacheMountPath},
		}
		r.job.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyNever
		r.job.Spec.Template.Spec.ServiceAccountName = r.serviceAccount.Name
		r.job.Spec.Template.Spec.Volumes = []corev1.Volume{
			{Name: dataVolumeName, VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: r.PVC.Name,
				}},
			},
			{Name: resticCache, VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: r.resticCache.Name,
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
	// We only continue reconciling if the restic job has completed
	return r.job.Status.Succeeded == 1, nil
}

//nolint:dupl
func (r *resticDestReconciler) cleanupJob(l logr.Logger) (bool, error) {
	logger := l.WithValues("job", r.job)
	// update time/duration
	if _, err := updateLastSyncDestination(r.Instance, r.scribeMetrics, logger); err != nil {
		return false, err
	}
	if r.job.Status.StartTime != nil {
		d := r.Instance.Status.LastSyncTime.Sub(r.job.Status.StartTime.Time)
		r.Instance.Status.LastSyncDuration = &metav1.Duration{Duration: d}
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

//nolint:dupl
func (r *resticDestReconciler) validateResticSpec(l logr.Logger) (bool, error) {
	var err error
	var result bool = true
	if len(r.Instance.Spec.Restic.Repository) == 0 {
		err = errors.New("Unable to get restic repository configurations")
		l.V(1).Info("Unable to get restic repository configurations")
		result = false
	}
	if err == nil {
		// get secret from cluster
		foundSecret := &corev1.Secret{}
		secretNotFoundErr := r.Client.Get(r.Ctx,
			types.NamespacedName{
				Name:      r.Instance.Spec.Restic.Repository,
				Namespace: r.Instance.Namespace,
			}, foundSecret)
		if secretNotFoundErr != nil {
			l.Error(err, "restic repository secret not found.",
				"repository", r.Instance.Spec.Restic.Repository)
			result = false
			err = secretNotFoundErr
		} else {
			r.resticRepositorySecret = foundSecret
		}
	}
	return result, err
}
