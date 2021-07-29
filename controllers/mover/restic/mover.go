/*
Copyright 2021 The VolSync authors.

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

package restic

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1beta1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/controllers/mover"
	"github.com/backube/volsync/controllers/utils"
	"github.com/backube/volsync/controllers/volumehandler"
)

const (
	resticCacheMountPath = "/cache"
	mountPath            = "/data"
	dataVolumeName       = "data"
	resticCache          = "cache"
)

// Mover is the reconciliation logic for the Restic-based data mover.
type Mover struct {
	client                client.Client
	logger                logr.Logger
	owner                 metav1.Object
	vh                    *volumehandler.VolumeHandler
	cacheAccessModes      []v1.PersistentVolumeAccessMode
	cacheCapacity         *resource.Quantity
	cacheStorageClassName *string
	repositoryName        string
	isSource              bool
	paused                bool
	mainPVCName           *string
	// Source-only fields
	pruneInterval *int32
	retainPolicy  *volsyncv1alpha1.ResticRetainPolicy
	sourceStatus  *volsyncv1alpha1.ReplicationSourceResticStatus
}

var _ mover.Mover = &Mover{}

// All object types that are temporary/per-iteration should be listed here. The
// individual objects to be cleaned up must also be marked.
var cleanupTypes = []client.Object{
	&v1.PersistentVolumeClaim{},
	&snapv1.VolumeSnapshot{},
	&batchv1.Job{},
}

func (m *Mover) Name() string { return "restic" }

func (m *Mover) Synchronize(ctx context.Context) (mover.Result, error) {
	var err error
	// Allocate temporary data PVC
	var dataPVC *v1.PersistentVolumeClaim
	if m.isSource {
		dataPVC, err = m.ensureSourcePVC(ctx)
	} else {
		dataPVC, err = m.ensureDestinationPVC(ctx)
	}
	if dataPVC == nil || err != nil {
		return mover.InProgress(), err
	}

	// Allocate cache volume
	cachePVC, err := m.ensureCache(ctx, dataPVC)
	if cachePVC == nil || err != nil {
		return mover.InProgress(), err
	}

	// Prepare ServiceAccount
	sa, err := m.ensureSA(ctx)
	if sa == nil || err != nil {
		return mover.InProgress(), err
	}

	// Validate Repository Secret
	repo, err := m.validateRepository(ctx)
	if repo == nil || err != nil {
		return mover.InProgress(), err
	}

	// Start mover Job
	job, err := m.ensureJob(ctx, cachePVC, dataPVC, sa, repo)
	if job == nil || err != nil {
		return mover.InProgress(), err
	}

	// On the destination, preserve the image and return it
	if !m.isSource {
		image, err := m.vh.EnsureImage(ctx, m.logger, dataPVC)
		if image == nil || err != nil {
			return mover.InProgress(), err
		}
		return mover.CompleteWithImage(image), nil
	}

	// On the source, just signal completion
	return mover.Complete(), nil
}

func (m *Mover) Cleanup(ctx context.Context) (mover.Result, error) {
	err := utils.CleanupObjects(ctx, m.client, m.logger, m.owner, cleanupTypes)
	if err != nil {
		return mover.InProgress(), err
	}
	return mover.Complete(), nil
}

func (m *Mover) ensureCache(ctx context.Context,
	dataPVC *v1.PersistentVolumeClaim) (*v1.PersistentVolumeClaim, error) {
	// Create a separate vh for the Restic cache volume that's based on the main
	// vh, but override options where necessary.
	cacheConfig := []volumehandler.VHOption{
		// build on the datavolume's configuration
		volumehandler.From(m.vh),
	}

	// Cache capacity defaults to 1Gi but can be overridden
	cacheCapacity := resource.MustParse("1Gi")
	if m.cacheCapacity != nil {
		cacheCapacity = *m.cacheCapacity
	}
	cacheConfig = append(cacheConfig, volumehandler.Capacity(&cacheCapacity))

	// AccessModes are generated in the following priority:
	// 1. Directly specified cache accessMode
	// 2. Directly specified volume accessMode
	// 3. Inherited from the source/data PVC
	if m.cacheAccessModes != nil {
		cacheConfig = append(cacheConfig, volumehandler.AccessModes(m.cacheAccessModes))
	} else if len(m.vh.GetAccessModes()) == 0 {
		cacheConfig = append(cacheConfig, volumehandler.AccessModes(dataPVC.Spec.AccessModes))
	}

	if m.cacheStorageClassName != nil {
		cacheConfig = append(cacheConfig, volumehandler.StorageClassName(m.cacheStorageClassName))
	}

	cacheVh, err := volumehandler.NewVolumeHandler(cacheConfig...)
	if err != nil {
		return nil, err
	}

	// Allocate cache volume
	cacheName := "volsync-" + m.owner.GetName() + "-cache"
	m.logger.Info("allocating cache volume", "PVC", cacheName)
	return cacheVh.EnsureNewPVC(ctx, m.logger, cacheName)
}

func (m *Mover) ensureSourcePVC(ctx context.Context) (*v1.PersistentVolumeClaim, error) {
	srcPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      *m.mainPVCName,
			Namespace: m.owner.GetNamespace(),
		},
	}
	if err := m.client.Get(ctx, utils.NameFor(srcPVC), srcPVC); err != nil {
		return nil, err
	}
	dataName := "volsync-" + m.owner.GetName() + "-src"
	return m.vh.EnsurePVCFromSrc(ctx, m.logger, srcPVC, dataName, true)
}

func (m *Mover) ensureDestinationPVC(ctx context.Context) (*v1.PersistentVolumeClaim, error) {
	if m.mainPVCName == nil {
		// Need to allocate the incoming data volume
		dataPVCName := "volsync-" + m.owner.GetName() + "-dest"
		return m.vh.EnsureNewPVC(ctx, m.logger, dataPVCName)
	}

	// use provided PVC
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      *m.mainPVCName,
			Namespace: m.owner.GetNamespace(),
		},
	}
	err := m.client.Get(ctx, utils.NameFor(pvc), pvc)
	return pvc, err
}

func (m *Mover) ensureSA(ctx context.Context) (*v1.ServiceAccount, error) {
	dir := "src"
	if !m.isSource {
		dir = "dst"
	}
	sa := &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "volsync-" + dir + "-" + m.owner.GetName(),
			Namespace: m.owner.GetNamespace(),
		},
	}
	saDesc := utils.NewSAHandler(ctx, m.client, m.owner, sa)
	cont, err := saDesc.Reconcile(m.logger)
	if cont {
		return sa, err
	}
	return nil, err
}

func (m *Mover) validateRepository(ctx context.Context) (*v1.Secret, error) {
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.repositoryName,
			Namespace: m.owner.GetNamespace(),
		},
	}
	logger := m.logger.WithValues("repositorySecret", utils.NameFor(secret))
	if err := utils.GetAndValidateSecret(ctx, m.client, logger, secret,
		"RESTIC_REPOSITORY", "RESTIC_PASSWORD"); err != nil {
		logger.Error(err, "Restic config secret does not contain the proper fields")
		return nil, err
	}
	return secret, nil
}

//nolint:funlen
func (m *Mover) ensureJob(ctx context.Context, cachePVC *v1.PersistentVolumeClaim,
	dataPVC *v1.PersistentVolumeClaim, sa *v1.ServiceAccount, repo *v1.Secret) (*batchv1.Job, error) {
	dir := "src"
	if !m.isSource {
		dir = "dst"
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "volsync-" + dir + "-" + m.owner.GetName(),
			Namespace: m.owner.GetNamespace(),
		},
	}
	logger := m.logger.WithValues("job", utils.NameFor(job))
	_, err := ctrlutil.CreateOrUpdate(ctx, m.client, job, func() error {
		if err := ctrl.SetControllerReference(m.owner, job, m.client.Scheme()); err != nil {
			logger.Error(err, "unable to set controller reference")
			return err
		}
		utils.MarkForCleanup(m.owner, job)
		job.Spec.Template.ObjectMeta.Name = job.Name
		backoffLimit := int32(8)
		job.Spec.BackoffLimit = &backoffLimit
		parallelism := int32(1)
		if m.paused {
			parallelism = int32(0)
		}
		job.Spec.Parallelism = &parallelism
		forgetOptions := generateForgetOptions(m.retainPolicy)
		runAsUser := int64(0)

		var actions []string
		if m.isSource {
			actions = []string{"backup"}
			if m.shouldPrune(time.Now()) {
				actions = append(actions, "prune")
			}
		} else {
			actions = []string{"restore"}
		}
		logger.Info("job actions", "actions", actions)

		job.Spec.Template.Spec.Containers = []v1.Container{{
			Name: "restic",
			Env: []v1.EnvVar{
				{Name: "FORGET_OPTIONS", Value: forgetOptions},
				{Name: "DATA_DIR", Value: mountPath},
				{Name: "RESTIC_CACHE_DIR", Value: resticCacheMountPath},
				// We populate environment variables from the restic repo
				// Secret. They are taken 1-for-1 from the Secret into env vars.
				// The allowed variables are defined by restic.
				// https://restic.readthedocs.io/en/stable/040_backup.html#environment-variables
				// Mandatory variables are needed to define the repository
				// location and its password.
				utils.EnvFromSecret(repo.Name, "RESTIC_REPOSITORY", false),
				utils.EnvFromSecret(repo.Name, "RESTIC_PASSWORD", false),
				// Optional variables based on what backend is used for restic
				utils.EnvFromSecret(repo.Name, "AWS_ACCESS_KEY_ID", true),
				utils.EnvFromSecret(repo.Name, "AWS_SECRET_ACCESS_KEY", true),
				utils.EnvFromSecret(repo.Name, "AWS_DEFAULT_REGION", true),
				utils.EnvFromSecret(repo.Name, "ST_AUTH", true),
				utils.EnvFromSecret(repo.Name, "ST_USER", true),
				utils.EnvFromSecret(repo.Name, "ST_KEY", true),
				utils.EnvFromSecret(repo.Name, "OS_AUTH_URL", true),
				utils.EnvFromSecret(repo.Name, "OS_REGION_NAME", true),
				utils.EnvFromSecret(repo.Name, "OS_USERNAME", true),
				utils.EnvFromSecret(repo.Name, "OS_USER_ID", true),
				utils.EnvFromSecret(repo.Name, "OS_PASSWORD", true),
				utils.EnvFromSecret(repo.Name, "OS_TENANT_ID", true),
				utils.EnvFromSecret(repo.Name, "OS_TENANT_NAME", true),
				utils.EnvFromSecret(repo.Name, "OS_USER_DOMAIN_NAME", true),
				utils.EnvFromSecret(repo.Name, "OS_USER_DOMAIN_ID", true),
				utils.EnvFromSecret(repo.Name, "OS_PROJECT_NAME", true),
				utils.EnvFromSecret(repo.Name, "OS_PROJECT_DOMAIN_NAME", true),
				utils.EnvFromSecret(repo.Name, "OS_PROJECT_DOMAIN_ID", true),
				utils.EnvFromSecret(repo.Name, "OS_TRUST_ID", true),
				utils.EnvFromSecret(repo.Name, "OS_APPLICATION_CREDENTIAL_ID", true),
				utils.EnvFromSecret(repo.Name, "OS_APPLICATION_CREDENTIAL_NAME", true),
				utils.EnvFromSecret(repo.Name, "OS_APPLICATION_CREDENTIAL_SECRET", true),
				utils.EnvFromSecret(repo.Name, "OS_STORAGE_URL", true),
				utils.EnvFromSecret(repo.Name, "OS_AUTH_TOKEN", true),
				utils.EnvFromSecret(repo.Name, "B2_ACCOUNT_ID", true),
				utils.EnvFromSecret(repo.Name, "B2_ACCOUNT_KEY", true),
				utils.EnvFromSecret(repo.Name, "AZURE_ACCOUNT_NAME", true),
				utils.EnvFromSecret(repo.Name, "AZURE_ACCOUNT_KEY", true),
				utils.EnvFromSecret(repo.Name, "GOOGLE_PROJECT_ID", true),
				utils.EnvFromSecret(repo.Name, "GOOGLE_APPLICATION_CREDENTIALS", true),
			},
			Command: []string{"/entry.sh"},
			Args:    actions,
			Image:   resticContainerImage,
			SecurityContext: &corev1.SecurityContext{
				RunAsUser: &runAsUser,
			},
			VolumeMounts: []corev1.VolumeMount{
				{Name: dataVolumeName, MountPath: mountPath},
				{Name: resticCache, MountPath: resticCacheMountPath},
			},
		}}
		job.Spec.Template.Spec.RestartPolicy = v1.RestartPolicyNever
		job.Spec.Template.Spec.ServiceAccountName = sa.Name
		job.Spec.Template.Spec.Volumes = []v1.Volume{
			{Name: dataVolumeName, VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: dataPVC.Name,
				}},
			},
			{Name: resticCache, VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: cachePVC.Name,
				}},
			},
		}
		return nil
	})
	// If Job had failed, delete it so it can be recreated
	if job.Status.Failed >= *job.Spec.BackoffLimit {
		logger.Info("deleting job -- backoff limit reached")
		err = m.client.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground))
		return nil, err
	}
	if err != nil {
		logger.Error(err, "reconcile failed")
	}

	// Stop here if the job hasn't completed yet
	if job.Status.Succeeded == 0 {
		return nil, nil
	}

	logger.Info("job completed")
	if m.isSource && m.shouldPrune(time.Now()) {
		now := metav1.Now()
		m.sourceStatus.LastPruned = &now
		logger.Info("prune completed", ".Status.Restic.LastPruned", m.sourceStatus.LastPruned)
	}
	// We only continue reconciling if the restic job has completed
	return job, nil
}

func (m *Mover) shouldPrune(current time.Time) bool {
	delta := time.Hour * 24 * 7 // default prune every 7 days
	if m.pruneInterval != nil {
		delta = time.Hour * 24 * time.Duration(*m.pruneInterval)
	}
	// If we've never pruned, the 1st one should be "delta" after creation.
	lastPruned := m.owner.GetCreationTimestamp().Time
	if !m.sourceStatus.LastPruned.IsZero() {
		lastPruned = m.sourceStatus.LastPruned.Time
	}
	return current.After(lastPruned.Add(delta))
}

func generateForgetOptions(policy *volsyncv1alpha1.ResticRetainPolicy) string {
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
