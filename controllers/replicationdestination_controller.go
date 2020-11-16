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
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/go-logr/logr"
	snapv1 "github.com/kubernetes-csi/external-snapshotter/v2/pkg/apis/volumesnapshot/v1beta1"
	"github.com/operator-framework/operator-lib/status"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
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
	// DefaultRsyncContainerImage is the default container image name of the rsync data mover
	DefaultRsyncContainerImage = "quay.io/backube/scribe-mover-rsync:latest"
)

// RsyncContainerImage is the container image name of the rsync data mover
var RsyncContainerImage string

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

	var result ctrl.Result
	var err error
	// Only reconcile if the replication method is internal
	if inst.Spec.Rsync != nil {
		result, err = RunRsyncDestReconciler(ctx, inst, r, logger)
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
	destinationVolumeHandler
	service    *corev1.Service
	mainSecret *corev1.Secret
	destSecret *corev1.Secret
	srcSecret  *corev1.Secret
	job        *batchv1.Job
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

	svcName := types.NamespacedName{
		Name:      "scribe-rsync-dest-" + r.Instance.Name,
		Namespace: r.Instance.Namespace,
	}
	logger := l.WithValues("service", svcName)

	r.service = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcName.Name,
			Namespace: svcName.Namespace,
		},
	}

	op, err := ctrlutil.CreateOrUpdate(r.Ctx, r.Client, r.service, func() error {
		if err := ctrl.SetControllerReference(r.Instance, r.service, r.Scheme); err != nil {
			logger.Error(err, "unable to set controller reference")
			return err
		}
		r.service.ObjectMeta.Annotations = map[string]string{
			"service.beta.kubernetes.io/aws-load-balancer-type": "nlb",
		}
		if r.Instance.Spec.Rsync.ServiceType != nil {
			r.service.Spec.Type = *r.Instance.Spec.Rsync.ServiceType
		} else {
			r.service.Spec.Type = corev1.ServiceTypeClusterIP
		}
		r.service.Spec.Selector = r.serviceSelector()
		if len(r.service.Spec.Ports) != 1 {
			r.service.Spec.Ports = []corev1.ServicePort{{}}
		}
		r.service.Spec.Ports[0].Name = "ssh"
		if r.Instance.Spec.Rsync.Port != nil {
			r.service.Spec.Ports[0].Port = *r.Instance.Spec.Rsync.Port
		} else {
			r.service.Spec.Ports[0].Port = 22
		}
		r.service.Spec.Ports[0].Protocol = corev1.ProtocolTCP
		r.service.Spec.Ports[0].TargetPort = intstr.FromInt(22)
		if r.service.Spec.Type == corev1.ServiceTypeClusterIP {
			r.service.Spec.Ports[0].NodePort = 0
		}
		return nil
	})
	if err != nil {
		logger.Error(err, "Service reconcile failed")
		return false, err
	}

	logger.V(1).Info("Service reconciled", "operation", op)
	return true, nil
}

func (r *rsyncDestReconciler) publishSvcAddress(l logr.Logger) (bool, error) {
	if r.service == nil { // no service, nothing to do
		return true, nil
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
	if address == "" {
		// We don't have an address yet, try again later
		r.Instance.Status.Rsync.Address = nil
		return false, nil
	}
	r.Instance.Status.Rsync.Address = &address

	l.V(1).Info("Service addr published", "address", address)
	return true, nil
}

func (r *rsyncDestReconciler) validateProvidedSSHKeys(l logr.Logger) (bool, error) {
	r.destSecret = &corev1.Secret{}
	secretName := types.NamespacedName{Name: *r.Instance.Spec.Rsync.SSHKeys, Namespace: r.Instance.Namespace}
	err := r.Client.Get(r.Ctx, secretName, r.destSecret)
	if err != nil {
		l.Error(err, "failed to get SSH keys Secret with provided name", "Secret", secretName)
		return false, err
	}
	for _, f := range []string{"destination", "destination.pub", "source.pub"} {
		if _, ok := r.destSecret.Data[f]; !ok {
			err = fmt.Errorf("field not found")
			l.Error(err, "SSH keys Secret is missing a required field", "field", f)
			return false, err
		}
	}
	return true, nil
}

func (r *rsyncDestReconciler) ensureSecrets(l logr.Logger) (bool, error) {
	// If user provided keys, use those
	if r.Instance.Spec.Rsync.SSHKeys != nil {
		return r.validateProvidedSSHKeys(l)
	}
	// Otherwise, create the secrets
	return reconcileBatch(l,
		r.ensureMainSecret,
		r.ensureConnectionSecret,
		r.ensureDestinationSecret,
	)
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
		Name:      "scribe-rsync-main-" + r.Instance.Name,
		Namespace: r.Instance.Namespace,
	}
	logger := l.WithValues("mainSecret", secName)

	// See if it exists and has the proper fields
	r.mainSecret = &corev1.Secret{}
	err := r.Client.Get(r.Ctx, secName, r.mainSecret)
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
			if err = r.Client.Delete(r.Ctx, r.mainSecret); err != nil {
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
	if err = r.Client.Create(r.Ctx, r.mainSecret); err != nil {
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
	if err := ctrl.SetControllerReference(r.Instance, r.mainSecret, r.Scheme); err != nil {
		l.Error(err, "unable to set controller reference")
		return err
	}

	sourceKeyFile := "/tmp/" + r.mainSecret.Namespace + "-" + r.mainSecret.Name + "-" + "source"
	priv, pub, err := generateKeyPair(r.Ctx, sourceKeyFile)
	if err != nil {
		l.Error(err, "unable to generate source ssh keys")
		return err
	}
	r.mainSecret.Data["source"] = priv
	r.mainSecret.Data["source.pub"] = pub

	destinationKeyFile := "/tmp/" + r.mainSecret.Namespace + "-" + r.mainSecret.Name + "-" + "destination"
	priv, pub, err = generateKeyPair(r.Ctx, destinationKeyFile)
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
	destName := types.NamespacedName{Name: "scribe-rsync-dest-" + r.Instance.Name, Namespace: r.Instance.Namespace}
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
	op, err := ctrlutil.CreateOrUpdate(r.Ctx, r.Client, r.destSecret, func() error {
		if err := ctrl.SetControllerReference(r.Instance, r.destSecret, r.Scheme); err != nil {
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
	srcName := types.NamespacedName{Name: "scribe-rsync-source-" + r.Instance.Name, Namespace: r.Instance.Namespace}
	logger := l.WithValues("sourceSecret", srcName)

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
	op, err := ctrlutil.CreateOrUpdate(r.Ctx, r.Client, r.srcSecret, func() error {
		if err := ctrl.SetControllerReference(r.Instance, r.srcSecret, r.Scheme); err != nil {
			logger.Error(err, "unable to set controller reference")
			return err
		}
		if r.srcSecret.Data == nil {
			r.srcSecret.Data = map[string][]byte{}
		}
		for _, k := range keysToCopy {
			r.srcSecret.Data[k] = r.mainSecret.Data[k]
		}
		return nil
	})

	if err != nil {
		r.Instance.Status.Rsync.SSHKeys = nil
		logger.Error(err, "reconcile failed")
	} else {
		r.Instance.Status.Rsync.SSHKeys = &srcName.Name
		logger.V(1).Info("Secret reconciled", "operation", op)
	}
	return true, err
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

func (r *rsyncDestReconciler) cleanupJob(l logr.Logger) (bool, error) {
	logger := l.WithValues("job", r.job)

	// Delete the (completed) Job. The next reconcile pass will recreate it.
	// Set propagation policy so the old pods get deleted
	if err := r.Client.Delete(r.Ctx, r.job, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
		logger.Error(err, "unable to delete old job")
		return false, err
	}

	return true, nil
}
