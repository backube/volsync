//go:build !disable_kopia

/*
Copyright 2024 The VolSync authors.

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

package kopia

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	vserrors "github.com/backube/volsync/internal/controller/errors"
	"github.com/backube/volsync/internal/controller/mover"
	"github.com/backube/volsync/internal/controller/utils"
	"github.com/backube/volsync/internal/controller/volumehandler"
)

const (
	kopiaCacheMountPath = "/cache"
	mountPath           = "/data"
	dataVolumeName      = "data"
	kopiaCache          = "cache"
	kopiaCAMountPath    = "/customCA"
	kopiaCAFilename     = "ca.crt"
	credentialDir       = "/credentials"
	gcsCredentialFile   = "gcs.json"
)

// Mover is the reconciliation logic for the Kopia-based data mover.
type Mover struct {
	client                client.Client
	logger                logr.Logger
	eventRecorder         events.EventRecorder
	owner                 client.Object
	vh                    *volumehandler.VolumeHandler
	saHandler             utils.SAHandler
	containerImage        string
	cacheAccessModes      []corev1.PersistentVolumeAccessMode
	cacheCapacity         *resource.Quantity
	cacheStorageClassName *string
	repositoryName        string
	isSource              bool
	paused                bool
	mainPVCName           *string
	customCASpec          volsyncv1alpha1.CustomCASpec
	privileged            bool
	latestMoverStatus     *volsyncv1alpha1.MoverStatus
	moverConfig           volsyncv1alpha1.MoverConfig
	// Source-only fields
	maintenanceInterval *int32
	retainPolicy        *volsyncv1alpha1.KopiaRetainPolicy
	compression         string
	parallelism         *int32
	actions             *volsyncv1alpha1.KopiaActions
	sourceStatus        *volsyncv1alpha1.ReplicationSourceKopiaStatus
	// Destination-only fields
	restoreAsOf     *string
	shallow         *int32
	cleanupTempPVC  bool
	cleanupCachePVC bool
}

var _ mover.Mover = &Mover{}

// All object types that are temporary/per-iteration should be listed here. The
// individual objects to be cleaned up must also be marked.
var cleanupTypes = []client.Object{
	&corev1.PersistentVolumeClaim{},
	&snapv1.VolumeSnapshot{},
	&batchv1.Job{},
}

func (m *Mover) Name() string { return kopiaMoverName }

func (m *Mover) Synchronize(ctx context.Context) (mover.Result, error) {
	if m.paused {
		return mover.Complete(), nil
	}

	var err error
	// Allocate temporary data PVC
	var dataPVC *corev1.PersistentVolumeClaim
	if m.isSource {
		dataPVC, err = m.ensureSourcePVC(ctx)
	} else {
		dataPVC, err = m.ensureDestinationPVC(ctx)
	}
	if dataPVC == nil || err != nil {
		return mover.InProgress(), err
	}

	// Allocate cache volume
	// cleanupCachePVC will always be false for replicationsources - it's only set in the builder FromDestination()
	cachePVC, err := m.ensureCache(ctx, dataPVC, m.cleanupCachePVC)
	if cachePVC == nil || err != nil {
		return mover.InProgress(), err
	}

	// Prepare ServiceAccount
	sa, err := m.saHandler.Reconcile(ctx, m.logger)
	if sa == nil || err != nil {
		return mover.InProgress(), err
	}

	// Validate Repository Secret
	repo, err := m.validateRepository(ctx)
	if repo == nil || err != nil {
		return mover.InProgress(), err
	}

	// Validate custom CA if in spec
	customCAObj, err := utils.ValidateCustomCA(ctx, m.client, m.logger,
		m.owner.GetNamespace(), m.customCASpec)
	// nil customCAObj is ok (indicates we're not using a custom CA)
	if err != nil {
		return mover.InProgress(), err
	}

	// Start mover Job
	job, err := m.ensureJob(ctx, cachePVC, dataPVC, sa, repo, customCAObj)
	if job == nil || err != nil {
		return mover.InProgress(), err
	}

	// Handle post-job success tasks
	if utils.IsJobCompleted(job) {
		if utils.IsJobSuccessful(job) {
			// Handle successful job based on type
			if m.isSource {
				// Check if we need to run maintenance
				if m.shouldRunMaintenance() {
					m.logger.Info("Running repository maintenance")
					m.updateMaintenanceTime()
				}
			}

			// Update status
			m.latestMoverStatus.Result = volsyncv1alpha1.MoverResultSuccessful
			m.latestMoverStatus.Logs = utils.GetJobLogs(ctx, m.client, job, m.logger)

			if !m.isSource {
				// Return completed image for destinations
				image := &corev1.TypedLocalObjectReference{
					APIVersion: "v1",
					Kind:       "PersistentVolumeClaim",
					Name:       dataPVC.Name,
				}
				return mover.CompleteWithImage(image), nil
			}
			return mover.Complete(), nil
		} else {
			// Job failed
			m.latestMoverStatus.Result = volsyncv1alpha1.MoverResultFailed
			m.latestMoverStatus.Logs = utils.GetJobLogs(ctx, m.client, job, m.logger)
			m.logger.Error(nil, "job failed")
			return mover.InProgress(), errors.New("kopia mover job failed")
		}
	}

	// Job is still running
	return mover.InProgress(), nil
}

func (m *Mover) Cleanup(ctx context.Context) (mover.Result, error) {
	err := utils.CleanupObjects(ctx, m.client, m.logger, m.owner, cleanupTypes)
	if err != nil {
		m.logger.Error(err, "error removing temporary objects")
	}
	return mover.Complete(), err
}

func (m *Mover) ensureSourcePVC(ctx context.Context) (*corev1.PersistentVolumeClaim, error) {
	return m.vh.EnsureImage(ctx, m.logger)
}

func (m *Mover) ensureDestinationPVC(ctx context.Context) (*corev1.PersistentVolumeClaim, error) {
	return m.vh.EnsureImage(ctx, m.logger, m.cleanupTempPVC)
}

func (m *Mover) ensureCache(ctx context.Context, dataPVC *corev1.PersistentVolumeClaim,
	cleanupCache bool) (*corev1.PersistentVolumeClaim, error) {
	logger := m.logger.WithValues("function", "ensureCache")

	// If we have cache settings, create cache volume
	if m.cacheCapacity != nil {
		cache := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "volsync-kopia-cache",
				Namespace: m.owner.GetNamespace(),
			},
		}
		utils.MarkForCleanup(m.owner, cache, cleanupCache)
		if _, err := ctrlutil.CreateOrUpdate(ctx, m.client, cache, func() error {
			if err := ctrl.SetControllerReference(m.owner, cache, m.client.Scheme()); err != nil {
				logger.Error(err, utils.ErrUnableToSetControllerRef)
				return err
			}
			utils.SetOwnedByVolSync(cache)
			cache.Spec.AccessModes = m.cacheAccessModes
			if len(cache.Spec.AccessModes) == 0 {
				cache.Spec.AccessModes = dataPVC.Spec.AccessModes
			}
			cache.Spec.Resources = corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					"storage": *m.cacheCapacity,
				},
			}
			if m.cacheStorageClassName != nil {
				cache.Spec.StorageClassName = m.cacheStorageClassName
			} else {
				cache.Spec.StorageClassName = dataPVC.Spec.StorageClassName
			}
			return nil
		}); err != nil {
			logger.Error(err, utils.ErrUnableToCreateOrUpdate, "resource", cache)
			return nil, err
		}
		return cache, nil
	}

	return nil, nil
}

func (m *Mover) validateRepository(ctx context.Context) (*corev1.Secret, error) {
	logger := m.logger.WithValues("function", "validateRepository")

	repository := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.repositoryName,
			Namespace: m.owner.GetNamespace(),
		},
	}
	if err := m.client.Get(ctx, client.ObjectKeyFromObject(repository), repository); err != nil {
		logger.Error(err, "unable to get repository secret", "secretName", m.repositoryName)
		return nil, err
	}

	// Validate required fields based on repository type
	if _, ok := repository.Data["KOPIA_PASSWORD"]; !ok {
		err := fmt.Errorf("KOPIA_PASSWORD not found in repository secret")
		logger.Error(err, "repository secret validation failed")
		return nil, err
	}

	return repository, nil
}

func (m *Mover) ensureJob(ctx context.Context, cachePVC *corev1.PersistentVolumeClaim,
	dataPVC *corev1.PersistentVolumeClaim, sa *corev1.ServiceAccount,
	repo *corev1.Secret, customCAObj client.Object) (*batchv1.Job, error) {
	logger := m.logger.WithValues("function", "ensureJob")

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "volsync-kopia-" + m.owner.GetName(),
			Namespace: m.owner.GetNamespace(),
		},
	}

	utils.MarkForCleanup(m.owner, job, true)

	if _, err := ctrlutil.CreateOrUpdate(ctx, m.client, job, func() error {
		if err := ctrl.SetControllerReference(m.owner, job, m.client.Scheme()); err != nil {
			logger.Error(err, utils.ErrUnableToSetControllerRef)
			return err
		}

		utils.SetOwnedByVolSync(job)
		jobTemplate := m.buildJob(cachePVC, dataPVC, sa, repo, customCAObj)
		job.Spec = jobTemplate.Spec
		return nil
	}); err != nil {
		logger.Error(err, utils.ErrUnableToCreateOrUpdate, "resource", job)
		return nil, err
	}

	return job, nil
}

func (m *Mover) buildJob(cachePVC *corev1.PersistentVolumeClaim,
	dataPVC *corev1.PersistentVolumeClaim, sa *corev1.ServiceAccount,
	repo *corev1.Secret, customCAObj client.Object) *batchv1.Job {

	volumes := []corev1.Volume{
		{
			Name: dataVolumeName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: dataPVC.Name,
				},
			},
		},
		{
			Name: "kopia-secret",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: repo.Name,
				},
			},
		},
	}

	volumeMounts := []corev1.VolumeMount{
		{Name: dataVolumeName, MountPath: mountPath},
		{Name: "kopia-secret", MountPath: credentialDir},
	}

	// Add cache volume if specified
	if cachePVC != nil {
		volumes = append(volumes, corev1.Volume{
			Name: kopiaCache,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: cachePVC.Name,
				},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      kopiaCache,
			MountPath: kopiaCacheMountPath,
		})
	}

	// Add custom CA if specified
	if customCAObj != nil {
		volumes = append(volumes, utils.CreateCustomCAVolume(customCAObj))
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      utils.CustomCAVolumeName,
			MountPath: kopiaCAMountPath,
			ReadOnly:  true,
		})
	}

	env := []corev1.EnvVar{
		{Name: "KOPIA_LOG_LEVEL", Value: "info"},
		{Name: "DATA_DIR", Value: mountPath},
		{Name: "CACHE_DIR", Value: kopiaCacheMountPath},
		{Name: "METRICS_PUSH_GATEWAY", Value: utils.GetMetricsPushGateway(m.owner.GetNamespace())},
		{Name: "VOLSYNC_MOVER_TYPE", Value: kopiaMoverName},
	}

	// Add source/destination specific environment variables
	if m.isSource {
		env = append(env, corev1.EnvVar{Name: "DIRECTION", Value: "source"})
		if m.compression != "" {
			env = append(env, corev1.EnvVar{Name: "KOPIA_COMPRESSION", Value: m.compression})
		}
		if m.parallelism != nil {
			env = append(env, corev1.EnvVar{Name: "KOPIA_PARALLELISM", Value: strconv.Itoa(int(*m.parallelism))})
		}
		// Add retention policy
		if m.retainPolicy != nil {
			if m.retainPolicy.Hourly != nil {
				env = append(env, corev1.EnvVar{Name: "KOPIA_RETAIN_HOURLY", Value: strconv.Itoa(int(*m.retainPolicy.Hourly))})
			}
			if m.retainPolicy.Daily != nil {
				env = append(env, corev1.EnvVar{Name: "KOPIA_RETAIN_DAILY", Value: strconv.Itoa(int(*m.retainPolicy.Daily))})
			}
			if m.retainPolicy.Weekly != nil {
				env = append(env, corev1.EnvVar{Name: "KOPIA_RETAIN_WEEKLY", Value: strconv.Itoa(int(*m.retainPolicy.Weekly))})
			}
			if m.retainPolicy.Monthly != nil {
				env = append(env, corev1.EnvVar{Name: "KOPIA_RETAIN_MONTHLY", Value: strconv.Itoa(int(*m.retainPolicy.Monthly))})
			}
			if m.retainPolicy.Yearly != nil {
				env = append(env, corev1.EnvVar{Name: "KOPIA_RETAIN_YEARLY", Value: strconv.Itoa(int(*m.retainPolicy.Yearly))})
			}
		}
		// Add actions
		if m.actions != nil {
			if m.actions.BeforeSnapshot != "" {
				env = append(env, corev1.EnvVar{Name: "KOPIA_BEFORE_SNAPSHOT", Value: m.actions.BeforeSnapshot})
			}
			if m.actions.AfterSnapshot != "" {
				env = append(env, corev1.EnvVar{Name: "KOPIA_AFTER_SNAPSHOT", Value: m.actions.AfterSnapshot})
			}
		}
	} else {
		env = append(env, corev1.EnvVar{Name: "DIRECTION", Value: "destination"})
		if m.restoreAsOf != nil {
			env = append(env, corev1.EnvVar{Name: "KOPIA_RESTORE_AS_OF", Value: *m.restoreAsOf})
		}
		if m.shallow != nil {
			env = append(env, corev1.EnvVar{Name: "KOPIA_SHALLOW", Value: strconv.Itoa(int(*m.shallow))})
		}
	}

	container := corev1.Container{
		Name:            "kopia",
		Image:           m.containerImage,
		Command:         []string{"/entry.sh"},
		Env:             env,
		VolumeMounts:    volumeMounts,
		SecurityContext: &corev1.SecurityContext{},
	}

	if m.moverConfig.MoverResources != nil {
		container.Resources = *m.moverConfig.MoverResources
	}

	podSpec := corev1.PodSpec{
		Containers:    []corev1.Container{container},
		RestartPolicy: corev1.RestartPolicyNever,
		Volumes:       volumes,
	}

	if m.moverConfig.MoverSecurityContext != nil {
		podSpec.SecurityContext = m.moverConfig.MoverSecurityContext
	}

	if m.moverConfig.MoverAffinity != nil {
		podSpec.Affinity = m.moverConfig.MoverAffinity
	}

	if sa != nil {
		podSpec.ServiceAccountName = sa.Name
	}

	if m.privileged {
		container.SecurityContext.Privileged = ptr.To(true)
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "volsync-kopia-" + m.owner.GetName(),
			Namespace: m.owner.GetNamespace(),
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name: "volsync-kopia-" + m.owner.GetName(),
				},
				Spec: podSpec,
			},
		},
	}

	utils.SetJobPodLabels(job, m.moverConfig.MoverPodLabels)

	return job
}

func (m *Mover) shouldRunMaintenance() bool {
	if m.maintenanceInterval == nil || *m.maintenanceInterval <= 0 {
		return false
	}

	if m.sourceStatus == nil || m.sourceStatus.LastMaintenance == nil {
		return true
	}

	lastMaintenance := m.sourceStatus.LastMaintenance.Time
	nextMaintenance := lastMaintenance.Add(time.Duration(*m.maintenanceInterval) * 24 * time.Hour)

	return time.Now().After(nextMaintenance)
}

func (m *Mover) updateMaintenanceTime() {
	if m.sourceStatus != nil {
		now := metav1.NewTime(time.Now())
		m.sourceStatus.LastMaintenance = &now
	}
}