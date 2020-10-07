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
	"io/ioutil"
	"os"
	"os/exec"
	"time"

	"github.com/go-logr/logr"
	snapv1 "github.com/kubernetes-csi/external-snapshotter/v2/pkg/apis/volumesnapshot/v1beta1"
	"github.com/operator-framework/operator-lib/status"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	scribev1alpha1 "github.com/backube/scribe/api/v1alpha1"
)

const (
	// Time format for snapshot names and labels
	timeYYYYMMDDHHMMSS      = "20060102150405"
	rsyncSnapshotAnnotation = "scribe.backube/rsync-snapname"
)

// RsyncContainerImage is the container image name of the rsync data mover
var RsyncContainerImage string

// DefaultRsyncContainerImage is the default container image name of the rsync data mover
var DefaultRsyncContainerImage = "quay.io/backube/scribe-mover-rsync:latest"

// ReplicationDestinationReconciler reconciles a ReplicationDestination object
type ReplicationDestinationReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

//nolint:lll
//+kubebuilder:rbac:groups=scribe.backube,resources=replicationdestinations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=scribe.backube,resources=replicationdestinations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
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
	if inst.Status.MethodStatus == nil {
		inst.Status.MethodStatus = map[string]string{}
	}

	var result ctrl.Result
	var err error
	// Only reconcile if the replication method is internal
	if inst.Spec.ReplicationMethod == scribev1alpha1.ReplicationMethodRsync {
		result, err = (&rsyncDestReconciler{}).Run(ctx, inst, r, logger)
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
		Owns(&snapv1.VolumeSnapshot{}).
		Complete(r)
}

type rsyncDestReconciler struct {
	ctx        context.Context
	instance   *scribev1alpha1.ReplicationDestination
	r          *ReplicationDestinationReconciler
	service    *corev1.Service
	mainSecret *corev1.Secret
	destSecret *corev1.Secret
	srcSecret  *corev1.Secret
	pvc        *corev1.PersistentVolumeClaim
	job        *batchv1.Job
	snap       *snapv1.VolumeSnapshot
}

func (r *rsyncDestReconciler) Run(ctx context.Context, instance *scribev1alpha1.ReplicationDestination,
	dr *ReplicationDestinationReconciler, logger logr.Logger) (ctrl.Result, error) {
	// Initialize state for the reconcile pass
	r.ctx = ctx
	r.instance = instance
	r.r = dr

	l := logger.WithValues("method", "Rsync")

	// The reconcile functions return True if we should continue reconciling
	reconcileFuncs := []struct {
		f    func(logr.Logger) (bool, error)
		desc string
	}{
		{r.ensureService, "Ensure incoming service"},
		{r.ensureMainSecret, "Ensure main secret"},
		{r.ensureDestinationSecret, "Ensure destination secret"},
		{r.ensureConnectionSecret, "Ensure connection secret/info"},
		{r.ensureIncomingPvc, "Ensure PVC for incoming data"},
		{r.ensureJob, "Ensure mover Job exists"},
		{r.snapshotVolume, "Snapshot volume if synchronized"},
		{r.cleanupJob, "Clean up job & old snapshot"},
	}
	for _, f := range reconcileFuncs {
		ll := l.WithValues("step", f.desc)
		ll.V(1).Info("starting step")
		if cont, err := f.f(ll); !cont || err != nil {
			//return ctrl.Result{Requeue: true}, err
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *rsyncDestReconciler) serviceSelector() map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":      "dest-" + r.instance.Name,
		"app.kubernetes.io/component": "rsync-mover",
		"app.kubernetes.io/part-of":   "scribe",
	}
}

// ensureService maintains the Service that is used to connect to the
// destination rsync mover.
func (r *rsyncDestReconciler) ensureService(l logr.Logger) (bool, error) {
	svcName := types.NamespacedName{
		Name:      "scribe-rsync-dest-" + r.instance.Name,
		Namespace: r.instance.Namespace,
	}
	logger := l.WithValues("service", svcName)

	r.service = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcName.Name,
			Namespace: svcName.Namespace,
		},
	}

	op, err := ctrlutil.CreateOrUpdate(r.ctx, r.r.Client, r.service, func() error {
		r.service.ObjectMeta.Annotations = map[string]string{
			"service.beta.kubernetes.io/aws-load-balancer-type": "nlb",
		}
		st, found := r.instance.Spec.Parameters[scribev1alpha1.RsyncServiceTypeKey]
		if found {
			r.service.Spec.Type = corev1.ServiceType(st)
		} else {
			r.service.Spec.Type = corev1.ServiceTypeClusterIP
		}
		r.service.Spec.Selector = r.serviceSelector()
		if len(r.service.Spec.Ports) != 1 {
			r.service.Spec.Ports = []corev1.ServicePort{{}}
		}
		r.service.Spec.Ports[0].Name = "ssh"
		r.service.Spec.Ports[0].Port = 22
		r.service.Spec.Ports[0].Protocol = corev1.ProtocolTCP
		r.service.Spec.Ports[0].TargetPort = intstr.FromInt(22)
		if r.service.Spec.Type == corev1.ServiceTypeClusterIP {
			r.service.Spec.Ports[0].NodePort = 0
		}
		if err := ctrl.SetControllerReference(r.instance, r.service, r.r.Scheme); err != nil {
			logger.Error(err, "unable to set controller reference")
		}
		return nil
	})
	if err != nil {
		logger.Error(err, "Service reconcile failed")
	} else {
		logger.V(1).Info("Service reconciled", "operation", op)
	}
	return op == ctrlutil.OperationResultNone, err
}

func (r *rsyncDestReconciler) ensureMainSecret(l logr.Logger) (bool, error) {
	// The secrets hold the ssh key pairs to ensure mutual authentication of the
	// connection. The main secret holds both keys and is used ensure the source
	// & destination secrets remain consistent with each other.
	//
	// Since the key generation creates unique keys each time it's run, we can't
	// do much to reconcile the main secret. All we can do is:
	// - Create it if it doesn't exist
	// - Ensure the expected fields are present within
	secName := types.NamespacedName{
		Name:      "scribe-rsync-main-" + r.instance.Name,
		Namespace: r.instance.Namespace,
	}
	logger := l.WithValues("mainSecret", secName)

	// See if it exists and has the proper fields
	r.mainSecret = &corev1.Secret{}
	err := r.r.Client.Get(r.ctx, secName, r.mainSecret)
	if err != nil && !kerrors.IsNotFound(err) {
		logger.Error(err, "failed to get secret")
		return false, err
	}
	if err == nil { // found it, make sure it has the right fields
		valid := true
		data := r.mainSecret.Data
		if data == nil || len(data) != 4 {
			valid = false
		} else {
			for _, k := range []string{"source", "source.pub", "destination", "destination.pub"} {
				if _, found := data[k]; !found {
					valid = false
				}
			}
		}
		if !valid {
			logger.V(1).Info("deleting invalid secret")
			if err = r.r.Client.Delete(r.ctx, r.mainSecret); err != nil {
				logger.Error(err, "failed to delete secret")
			}
			return false, err
		}
		// Secret is valid, we're done
		logger.V(1).Info("secret is valid")
		return true, nil
	}

	// Need to create the secret
	r.mainSecret.Name = secName.Name
	r.mainSecret.Namespace = secName.Namespace
	if err = r.generateMainSecret(l); err != nil {
		l.Error(err, "unable to generate main secret")
		return false, err
	}
	if err = r.r.Client.Create(r.ctx, r.mainSecret); err != nil {
		l.Error(err, "unable to create secret")
		return false, err
	}

	l.V(1).Info("created secret")
	return false, nil
}

func generateKeyPair(ctx context.Context, filename string) (private []byte, public []byte, err error) {
	defer os.RemoveAll(filename)
	defer os.RemoveAll(filename + ".pub")
	if err = exec.CommandContext(ctx, "ssh-keygen", "-q", "-t", "rsa", "-b", "4096",
		"-f", filename, "-C", "", "-N", "").Run(); err != nil {
		return
	}
	if private, err = ioutil.ReadFile(filename); err != nil {
		return
	}
	public, err = ioutil.ReadFile(filename + ".pub")
	return
}

func (r *rsyncDestReconciler) generateMainSecret(l logr.Logger) error {
	r.mainSecret.Data = make(map[string][]byte, 4)
	if err := ctrl.SetControllerReference(r.instance, r.mainSecret, r.r.Scheme); err != nil {
		l.Error(err, "unable to set controller reference")
		return err
	}

	sourceKeyFile := "/tmp/" + r.mainSecret.Namespace + "-" + r.mainSecret.Name + "-" + "source"
	priv, pub, err := generateKeyPair(r.ctx, sourceKeyFile)
	if err != nil {
		l.Error(err, "unable to generate source ssh keys")
		return err
	}
	r.mainSecret.Data["source"] = priv
	r.mainSecret.Data["source.pub"] = pub

	destinationKeyFile := "/tmp/" + r.mainSecret.Namespace + "-" + r.mainSecret.Name + "-" + "destination"
	priv, pub, err = generateKeyPair(r.ctx, destinationKeyFile)
	if err != nil {
		l.Error(err, "unable to generate destination ssh keys")
		return err
	}
	r.mainSecret.Data["destination"] = priv
	r.mainSecret.Data["destination.pub"] = pub

	l.V(1).Info("created secret")
	return nil
}

func (r *rsyncDestReconciler) ensureDestinationSecret(l logr.Logger) (bool, error) {
	destName := types.NamespacedName{Name: "scribe-rsync-dest-" + r.instance.Name, Namespace: r.instance.Namespace}
	logger := l.WithValues("destSecret", destName)

	// The destination secret is a subset of the main secret
	keysToCopy := []string{"source.pub", "destination", "destination.pub"}
	for _, k := range keysToCopy {
		if _, ok := r.mainSecret.Data[k]; !ok {
			logger.V(1).Info("key not present in secret", "key", k)
			return false, nil
		}
	}

	r.destSecret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      destName.Name,
			Namespace: destName.Namespace,
		},
	}
	op, err := ctrlutil.CreateOrUpdate(r.ctx, r.r.Client, r.destSecret, func() error {
		if err := ctrl.SetControllerReference(r.instance, r.destSecret, r.r.Scheme); err != nil {
			logger.Error(err, "unable to set controller reference")
			return err
		}
		if r.destSecret.Data == nil {
			r.destSecret.Data = map[string][]byte{}
		}
		for _, k := range keysToCopy {
			r.destSecret.Data[k] = r.mainSecret.Data[k]
		}
		return nil
	})

	if err != nil {
		logger.Error(err, "reconcile failed")
	} else {
		logger.V(1).Info("Secret reconciled", "operation", op)
	}
	return op == ctrlutil.OperationResultNone, err
}

func (r *rsyncDestReconciler) ensureConnectionSecret(l logr.Logger) (bool, error) {
	srcName := types.NamespacedName{Name: "scribe-rsync-source-" + r.instance.Name, Namespace: r.instance.Namespace}
	logger := l.WithValues("srcSecret", srcName)

	// The source secret is a subset of the main secret
	keysToCopy := []string{"source", "source.pub", "destination.pub"}
	for _, k := range keysToCopy {
		if _, ok := r.mainSecret.Data[k]; !ok {
			logger.V(1).Info("key not present in secret", "key", k)
			return false, nil
		}
	}

	r.srcSecret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      srcName.Name,
			Namespace: srcName.Namespace,
		},
	}
	op, err := ctrlutil.CreateOrUpdate(r.ctx, r.r.Client, r.srcSecret, func() error {
		if err := ctrl.SetControllerReference(r.instance, r.srcSecret, r.r.Scheme); err != nil {
			logger.Error(err, "unable to set controller reference")
			return err
		}
		if r.srcSecret.Data == nil {
			r.srcSecret.Data = map[string][]byte{}
		}
		for _, k := range keysToCopy {
			r.srcSecret.Data[k] = r.mainSecret.Data[k]
		}
		address := r.service.Spec.ClusterIP
		if r.service.Spec.Type == corev1.ServiceTypeLoadBalancer {
			if len(r.service.Status.LoadBalancer.Ingress) > 0 {
				if r.service.Status.LoadBalancer.Ingress[0].Hostname != "" {
					address = r.service.Status.LoadBalancer.Ingress[0].Hostname
				} else if r.service.Status.LoadBalancer.Ingress[0].IP != "" {
					address = r.service.Status.LoadBalancer.Ingress[0].IP
				}
			}
		}
		r.srcSecret.Data["address"] = []byte(address)
		return nil
	})

	if err != nil {
		delete(r.instance.Status.MethodStatus, scribev1alpha1.RsyncConnectionInfoKey)
		logger.Error(err, "reconcile failed")
	} else {
		r.instance.Status.MethodStatus[scribev1alpha1.RsyncConnectionInfoKey] = srcName.Name
		logger.V(1).Info("Secret reconciled", "operation", op)
	}
	return op == ctrlutil.OperationResultNone, err
}

func (r *rsyncDestReconciler) ensureIncomingPvc(l logr.Logger) (bool, error) {
	pvcName := types.NamespacedName{Name: "scribe-rsync-dest-" + r.instance.Name, Namespace: r.instance.Namespace}
	logger := l.WithValues("PVC", pvcName)

	// Ensure required configuration parameters have been provided
	accessMode, ok := r.instance.Spec.Parameters[scribev1alpha1.RsyncAccessModeKey]
	if !ok {
		return false, errors.New("parameter " + scribev1alpha1.RsyncAccessModeKey + " must be provided")
	}
	pvcCapacityStr, ok := r.instance.Spec.Parameters[scribev1alpha1.RsyncCapacityKey]
	if !ok {
		return false, errors.New("parameter " + scribev1alpha1.RsyncCapacityKey + " must be provided")
	}
	pvcCapacity, err := resource.ParseQuantity(pvcCapacityStr)
	if err != nil {
		return false, errors.New("parameter " + scribev1alpha1.RsyncCapacityKey + " must be a resource quantity")
	}
	var storageClassName *string
	if scName, ok := r.instance.Spec.Parameters[scribev1alpha1.RsyncStorageClassNameKey]; ok {
		storageClassName = &scName
	}

	r.pvc = &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName.Name,
			Namespace: pvcName.Namespace,
		},
	}
	// Note: we don't reconcile the immutable fields. We could do it by deleting
	// and recreating the PVC.
	op, err := ctrlutil.CreateOrUpdate(r.ctx, r.r.Client, r.pvc, func() error {
		if err := ctrl.SetControllerReference(r.instance, r.pvc, r.r.Scheme); err != nil {
			logger.Error(err, "unable to set controller reference")
			return err
		}
		if r.pvc.CreationTimestamp.IsZero() { // set immutable fields
			r.pvc.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{
				corev1.PersistentVolumeAccessMode(accessMode),
			}
			r.pvc.Spec.StorageClassName = storageClassName
			volumeMode := corev1.PersistentVolumeFilesystem
			r.pvc.Spec.VolumeMode = &volumeMode
		}

		r.pvc.Spec.Resources.Requests = corev1.ResourceList{
			corev1.ResourceStorage: pvcCapacity,
		}
		return nil
	})

	if err != nil {
		logger.Error(err, "reconcile failed")
	} else {
		logger.V(1).Info("PVC reconciled", "operation", op)
	}
	return op == ctrlutil.OperationResultNone, err
}

//nolint:funlen
func (r *rsyncDestReconciler) ensureJob(l logr.Logger) (bool, error) {
	jobName := types.NamespacedName{
		Name:      "scribe-rsync-dest-" + r.instance.Name,
		Namespace: r.instance.Namespace,
	}
	logger := l.WithValues("job", jobName)

	r.job = &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName.Name,
			Namespace: jobName.Namespace,
		},
	}

	op, err := ctrlutil.CreateOrUpdate(r.ctx, r.r.Client, r.job, func() error {
		if err := ctrl.SetControllerReference(r.instance, r.job, r.r.Scheme); err != nil {
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
		if len(r.job.Spec.Template.Spec.Containers) != 1 {
			r.job.Spec.Template.Spec.Containers = []corev1.Container{{}}
		}
		r.job.Spec.Template.Spec.Containers[0].Name = "rsync"
		r.job.Spec.Template.Spec.Containers[0].Command = []string{"/bin/bash", "-c", "/destination.sh"}
		r.job.Spec.Template.Spec.Containers[0].Image = RsyncContainerImage
		runAsUser := int64(0)
		r.job.Spec.Template.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{
			Capabilities: &corev1.Capabilities{
				Add: []corev1.Capability{"AUDIT_WRITE"},
			},
			RunAsUser: &runAsUser,
		}
		r.job.Spec.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
			{Name: "data", MountPath: "/data"},
			{Name: "keys", MountPath: "/keys"},
		}
		r.job.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyNever
		secretMode := int32(0600)
		r.job.Spec.Template.Spec.Volumes = []corev1.Volume{
			{Name: "data", VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: r.pvc.Name,
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
		err = r.r.Client.Delete(r.ctx, r.job, client.PropagationPolicy(metav1.DeletePropagationBackground))
		return false, err
	}

	if err != nil {
		logger.Error(err, "reconcile failed")
	} else {
		logger.V(1).Info("Job reconciled", "operation", op)
	}
	return op == ctrlutil.OperationResultNone, err
}

func (r *rsyncDestReconciler) snapshotVolume(l logr.Logger) (bool, error) {
	// We only continue if the rsync job has completed
	if r.job.Status.Succeeded == 0 {
		return false, nil
	}

	// Track the name of the (in-progress) snapshot as a Job annotation
	snapName := types.NamespacedName{Namespace: r.instance.Namespace}
	if r.job.Annotations == nil {
		r.job.Annotations = make(map[string]string)
	}
	if name, ok := r.job.Annotations[rsyncSnapshotAnnotation]; ok {
		snapName.Name = name
	} else {
		ts := time.Now().Format(timeYYYYMMDDHHMMSS)
		snapName.Name = "scribe-rsync-dest-" + r.instance.Name + "-" + ts
		r.job.Annotations[rsyncSnapshotAnnotation] = snapName.Name
		if err := r.r.Client.Update(r.ctx, r.job); err != nil {
			l.Error(err, "unable to update job")
			return false, err
		}
	}
	logger := l.WithValues("snapshot", snapName)

	r.snap = &snapv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      snapName.Name,
			Namespace: snapName.Namespace,
		},
	}
	op, err := ctrlutil.CreateOrUpdate(r.ctx, r.r.Client, r.snap, func() error {
		if err := ctrl.SetControllerReference(r.instance, r.snap, r.r.Scheme); err != nil {
			logger.Error(err, "unable to set controller reference")
			return err
		}
		if r.snap.CreationTimestamp.IsZero() {
			r.snap.Spec = snapv1.VolumeSnapshotSpec{
				Source: snapv1.VolumeSnapshotSource{
					PersistentVolumeClaimName: &r.pvc.Name,
				},
			}
			if vscn, ok := r.instance.Spec.Parameters[scribev1alpha1.RsyncVolumeSnapshotClassNameKey]; ok {
				r.snap.Spec.VolumeSnapshotClassName = &vscn
			}
		}

		return nil
	})

	if err != nil {
		logger.Error(err, "reconcile failed")
	} else {
		logger.V(1).Info("Snapshot reconciled", "operation", op)
	}
	return op == ctrlutil.OperationResultNone, err
}

func (r *rsyncDestReconciler) cleanupJob(l logr.Logger) (bool, error) {
	logger := l.WithValues("job", r.job)

	// We only continue if the snapshot has been bound
	if r.snap.Status == nil || r.snap.Status.BoundVolumeSnapshotContentName == nil {
		return false, nil
	}

	// Delete the old snapshot (if it exists)
	oldSnap, ok := r.instance.Status.MethodStatus[scribev1alpha1.RsyncLatestSnapKey]
	if ok && oldSnap != r.snap.Name {
		// Delete the old snapshot
		old := &snapv1.VolumeSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      oldSnap,
				Namespace: r.instance.Namespace,
			},
		}
		logger.V(1).Info("deleting old snapshot", "old-snapshot", oldSnap)
		err := r.r.Client.Delete(r.ctx, old)
		if err != nil && !kerrors.IsNotFound(err) {
			logger.Error(err, "unable to delete old snapshot")
		}
	}

	// Delete succeeded, so move the current snapshot to be the "latest".
	// This also performs the CR status update here (in addition to at the
	// conclusion of the main reconcile loop). The reason it's done here is that
	// sometimes the update fails (object changed error), and that can cause the
	// update of the latest snapshot to be lost. Doing it here lets us terminate
	// the reconcile process and try again before the Job is deleted. Deleting
	// the Job is the commit point that signals the end of the current
	// replication cycle, so there's no chance to try again after that delete
	// happens.
	r.instance.Status.MethodStatus[scribev1alpha1.RsyncLatestSnapKey] = r.snap.Name
	err := r.r.Status().Update(r.ctx, r.instance)
	if err != nil {
		logger.Error(err, "unable to update instance status")
		return false, err
	}

	// Delete the (completed) Job. The next reconcile pass will recreate it.
	// Set propagation policy so the old pods get deleted
	if err := r.r.Client.Delete(r.ctx, r.job, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
		logger.Error(err, "unable to delete old job")
		return false, err
	}

	return true, nil
}
