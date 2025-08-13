/*
Copyright 2023 The VolSync authors.

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
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	volumepopulatorv1beta1 "github.com/kubernetes-csi/volume-data-source-validator/client/apis/volumepopulator/v1beta1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/component-helpers/storage/volume"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/internal/controller/utils"
)

const (
	populatorPvcPrefix      string = "vs-prime"
	annotationSelectedNode  string = "volume.kubernetes.io/selected-node"
	annotationPopulatedFrom string = "volsync.backube/populated-from"
	labelPvcPrime           string = utils.VolsyncLabelPrefix + "/populator-pvc-for"

	VolPopPVCToReplicationDestinationIndex string = "volPopPvc.spec.dataSourceRef.Name"
	VolPopPVCToStorageClassIndex           string = "volPopPvc.spec.storageClassName"

	VolPopCRName string = "volsync-replicationdestination"
)

func IndexFieldsForVolumePopulator(ctx context.Context, fieldIndexer client.FieldIndexer) error {
	// Index on PVCs - used to find pvc referring to (by dataSourceRef) a ReplicationDestination
	err := fieldIndexer.IndexField(ctx, &corev1.PersistentVolumeClaim{},
		VolPopPVCToReplicationDestinationIndex, func(o client.Object) []string {
			var res []string
			pvc, ok := o.(*corev1.PersistentVolumeClaim)
			if !ok {
				// This shouldn't happen
				return res
			}
			if !pvcHasReplicationDestinationDataSourceRef(pvc) {
				// This pvc is not using a ReplicationDestination as a DataSourceRef, don't add to index
				return res
			}

			// just return the raw field value -- the indexer will take care of dealing with namespaces for us
			res = append(res, pvc.Spec.DataSourceRef.Name)

			return res
		})
	if err != nil {
		return err
	}

	// Index on PVCs - used to find pvcs (for this volume populator) referring to a storageclass
	// Will only index PVCs that are using a ReplicationDestination as DataSourceRef
	return fieldIndexer.IndexField(ctx, &corev1.PersistentVolumeClaim{},
		VolPopPVCToStorageClassIndex, func(o client.Object) []string {
			var res []string
			pvc, ok := o.(*corev1.PersistentVolumeClaim)
			if !ok {
				// This shouldn't happen
				return res
			}
			if !pvcHasReplicationDestinationDataSourceRef(pvc) {
				// This pvc is not using a ReplicationDestination as a DataSourceRef, don't add to index
				return res
			}

			// just return the raw field value -- the indexer will take care of dealing with namespaces for us
			if pvc.Spec.StorageClassName != nil {
				res = append(res, *pvc.Spec.StorageClassName)
			}

			return res
		})
}

// If the VolumePopulator CRD is present (i.e. the VolumePopulator API is available), then make sure we have
// VolumePopulator CR to register VolSync ReplicationDestination as a valid VolumePopulator
func EnsureVolSyncVolumePopulatorCRIfCRDPresent(ctx context.Context,
	k8sClient client.Client, logger logr.Logger) error {
	logger = logger.WithValues("VolPopCRName", VolPopCRName)

	ok, err := isVolumePopulatorCRDPresent(ctx, k8sClient)
	if err != nil {
		return err
	}
	if !ok {
		logger.Info("VolumePopulator Kind is not present, " +
			"not creating a VolumePopulator CR")
		return nil // VolumePopulator kind is not present, nothing to do
	}

	volSyncVP := &volumepopulatorv1beta1.VolumePopulator{
		ObjectMeta: metav1.ObjectMeta{
			Name: VolPopCRName,
		},
	}
	op, err := ctrlutil.CreateOrUpdate(ctx, k8sClient, volSyncVP, func() error {
		volSyncVP.SourceKind = metav1.GroupKind{
			Group: volsyncv1alpha1.GroupVersion.Group,
			Kind:  "ReplicationDestination",
		}
		// Add VolSync label
		utils.SetOwnedByVolSync(volSyncVP)

		return nil
	})
	if err != nil {
		logger.Error(err, "Ensuring VolumePopulator failed")
		return err
	}

	if op == ctrlutil.OperationResultCreated {
		logger.Info("Created VolumePopulatorCR")
	}

	logger.Info("Ensuring VolumePopulator CR complete")
	return nil
}

func isVolumePopulatorCRDPresent(ctx context.Context, k8sClient client.Client) (bool, error) {
	vpList := volumepopulatorv1beta1.VolumePopulatorList{}
	err := k8sClient.List(ctx, &vpList)
	if err != nil {
		if utils.IsCRDNotPresentError(err) {
			// VolumePopulator Kind is not present
			return false, nil
		}
		// Some other error querying for VolumePopulators
		return false, err
	}
	return true, nil
}

//nolint:lll
//+kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=persistentvolumes,verbs=get;list;watch;patch
//+kubebuilder:rbac:groups=volsync.backube,resources=replicationdestinations,verbs=get;list;watch
//+kubebuilder:rbac:groups=storage.k8s.io,resources=storageclasses,verbs=get;list;watch
//+kubebuilder:rbac:groups=populator.storage.k8s.io,resources=volumepopulators,verbs=get;list;watch;create;update;patch

// VolumePopulatorReconciler reconciles PVCs that use a dataSourceRef that refers to a
// ReplicationDestination object.
// The VolumePopulatorReconciler will create a PVC from the latest snapshot image in
// a ReplicationDestination.
type VolumePopulatorReconciler struct {
	client.Client
	Log           logr.Logger
	Scheme        *runtime.Scheme
	EventRecorder record.EventRecorder
}

type vpResult struct {
	res ctrl.Result
	err error
}

func (vpr *vpResult) result() (ctrl.Result, error) {
	return vpr.res, vpr.err
}

// Reconcile logic is adapted from reference at:
// https://github.com/kubernetes-csi/lib-volume-populator/blob/master/populator-machinery/controller.go
func (r *VolumePopulatorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("pvc", req.NamespacedName)

	// Get PVC CR instance
	pvc := &corev1.PersistentVolumeClaim{}
	if err := r.Get(ctx, req.NamespacedName, pvc); err != nil {
		if !kerrors.IsNotFound(err) {
			logger.Error(err, "Failed to get PVC")
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check to make sure we should be reconciling this PVC - just in case
	if shouldReconcile := pvcHasReplicationDestinationDataSourceRef(pvc); !shouldReconcile {
		return ctrl.Result{}, nil
	}

	res, err := r.reconcilePVC(ctx, logger, pvc)
	if err != nil {
		logger.Error(err, "error reconciling PVC")
	}
	return res, err
}

//nolint:funlen
func (r *VolumePopulatorReconciler) reconcilePVC(ctx context.Context, logger logr.Logger,
	pvc *corev1.PersistentVolumeClaim) (ctrl.Result, error) {
	waitForFirstConsumer, nodeName, scResult := r.checkStorageClass(ctx, logger, pvc)
	if scResult != nil {
		return scResult.result()
	}

	// Look for PVC' - this will be a PVC with the dataSourceRef set to the latest snapshot image
	// from the ReplicationDestination
	// pvcPrime will be nil if not found
	pvcPrime, err := GetVolumePopulatorPVCPrime(ctx, r.Client, pvc)
	if err != nil {
		return ctrl.Result{}, err
	}

	// If the PVC is unbound, we need to perform the population
	if !isPVCBoundToVolume(pvc) {
		pvcPrime, primeResult := r.reconcilePVCPrime(ctx, logger, pvc, pvcPrime, waitForFirstConsumer, nodeName)
		if primeResult != nil {
			return primeResult.result()
		}

		// Make sure any snapshots we've tried to use have owner reference of pvcPrime (for future cleanup)
		err = r.ensureOwnerReferenceOnSnapshots(ctx, pvc, pvcPrime)
		if err != nil {
			return ctrl.Result{}, err
		}

		rbResult := r.rebindPVClaim(ctx, logger, pvc, pvcPrime)
		if rbResult != nil {
			return rbResult.result()
		}
	}

	// Wait for the bind controller to rebind the PV
	if pvcPrime != nil {
		if corev1.ClaimLost != pvcPrime.Status.Phase {
			logger.Info("Waiting for pv rebind", "from pvc", pvcPrime.GetName(), "to pvc", pvc.GetName())
			return ctrl.Result{}, nil
		}
	}

	// *** At this point the volume population is done and we're just cleaning up ***
	r.EventRecorder.Eventf(pvc, corev1.EventTypeNormal, volsyncv1alpha1.EvRVolPopPVCPopulatorFinished,
		"Populator finished")

	// Cleanup
	if err := r.cleanup(ctx, logger, pvc, pvcPrime); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *VolumePopulatorReconciler) checkStorageClass(ctx context.Context, logger logr.Logger,
	pvc *corev1.PersistentVolumeClaim) (bool, string, *vpResult) {
	var waitForFirstConsumer bool
	var nodeName string
	if pvc.Spec.StorageClassName != nil {
		storageClassName := *pvc.Spec.StorageClassName

		storageClass := &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: storageClassName,
			},
		}
		err := r.Get(ctx, client.ObjectKeyFromObject(storageClass), storageClass)
		if err != nil {
			if !kerrors.IsNotFound(err) {
				return false, "", &vpResult{ctrl.Result{}, err}
			}
			logger.Error(err, "StorageClass not found, cannot populate volume yet")
			// Do not return error - will rely on watches to reconcile once storageclass is created
			// No need for event here - storagecontroller adds warning if storageclass doesn't exist
			return false, "", &vpResult{ctrl.Result{}, nil}
		}

		if err := checkIntreeStorageClass(pvc, storageClass); err != nil {
			logger.Error(err, "Ignoring PVC")
			return false, "", &vpResult{ctrl.Result{}, nil}
		}

		if storageClass.VolumeBindingMode != nil &&
			storagev1.VolumeBindingWaitForFirstConsumer == *storageClass.VolumeBindingMode {
			waitForFirstConsumer = true
			nodeName = pvc.Annotations[annotationSelectedNode]
			if nodeName == "" {
				// Wait for the PVC to get a node name before continuing
				logger.Info("VolumeBindingMode is WaitForFirstConsumer, need to wait for nodeName annotation",
					"annotation name", annotationSelectedNode)
				return false, "", &vpResult{ctrl.Result{}, nil}
			}
		}
	}
	return waitForFirstConsumer, nodeName, nil
}

//nolint:funlen
func (r *VolumePopulatorReconciler) reconcilePVCPrime(ctx context.Context, logger logr.Logger,
	pvc, pvcPrime *corev1.PersistentVolumeClaim,
	waitForFirstConsumer bool, nodeName string) (*corev1.PersistentVolumeClaim, *vpResult) {
	if pvcPrime == nil {
		// pvcPrime doesn't exist yet
		// Check for existence of ReplicationDestination here - if PVC' was already there, then it may
		// be ok if replicationdestination is missing - so only error out here if RD doesn't exist
		rd, err := r.getReplicationDestinationFromDataSourceRef(ctx, logger, pvc)
		if err != nil {
			if !kerrors.IsNotFound(err) {
				return nil, &vpResult{ctrl.Result{}, err}
			}
			logger.Error(err, "ReplicationDestination not found, cannot populate volume yet")
			r.EventRecorder.Eventf(pvc, corev1.EventTypeWarning, volsyncv1alpha1.EvVolPopPVCReplicationDestMissing,
				"Unable to populate volume: %s", err)
			// Do not return error - will rely on watches to reconcile once the rd is created
			return nil, &vpResult{ctrl.Result{}, nil}
		}

		logger = logger.WithValues("replication destination name", rd.GetName(), "namespace", rd.GetNamespace())

		if rd.Status == nil || rd.Status.LatestImage == nil {
			logger.Info("ReplicationDestination has no latestImage, cannot populate volume yet")
			r.EventRecorder.Eventf(pvc, corev1.EventTypeWarning, volsyncv1alpha1.EvRVolPopPVCReplicationDestNoLatestImage,
				"Unable to populate volume, waiting for replicationdestination to have latestImage")
			// We'll get called again later when the replicationdestination is updated (see watches on repldest)
			return nil, &vpResult{ctrl.Result{}, nil}
		}

		latestImage := rd.Status.LatestImage

		if !utils.IsSnapshot(latestImage) {
			// This means the replicationdestination is using "Direct" (aka "None") CopyMethod
			dataSourceRefErr := fmt.Errorf("ReplicationDestination latestImage is not a volumesnapshot")
			logger.Error(dataSourceRefErr, "Unable to populate volume")
			r.EventRecorder.Eventf(pvc, corev1.EventTypeWarning, volsyncv1alpha1.EvRVolPopPVCPopulatorError,
				"Unable to populate volume: %s", dataSourceRefErr)
			// Do not return error here - no use retrying
			return nil, &vpResult{ctrl.Result{}, nil}
		}

		_, err = r.validateSnapshotAndLabel(ctx, logger, latestImage.Name, rd.GetNamespace(), pvc)
		if err != nil {
			return nil, &vpResult{ctrl.Result{}, err}
		}

		pvcPrime = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      getPVCPrimeName(pvc),
				Namespace: pvc.GetNamespace(),
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes:      pvc.Spec.AccessModes,
				Resources:        pvc.Spec.Resources,
				StorageClassName: pvc.Spec.StorageClassName,
				VolumeMode:       pvc.Spec.VolumeMode,
				DataSourceRef: &corev1.TypedObjectReference{
					APIGroup: latestImage.APIGroup,
					Kind:     latestImage.Kind,
					Name:     latestImage.Name,
					//Namespace: &rd.GetNamespace(), // Future, if we support cross-namespace
				},
			},
		}
		if waitForFirstConsumer {
			pvcPrime.Annotations = map[string]string{
				annotationSelectedNode: nodeName,
			}
		}
		// Make pvcPrime owned by pvc - will be cleaned up via gc if pvc is deleted
		if err := ctrl.SetControllerReference(pvc, pvcPrime, r.Client.Scheme()); err != nil {
			logger.Error(err, utils.ErrUnableToSetControllerRef)
			return nil, &vpResult{ctrl.Result{}, err}
		}
		utils.AddLabel(pvcPrime, labelPvcPrime, pvc.GetName()) // Use this filter in predicates in the &Owns() watcher
		utils.SetOwnedByVolSync(pvcPrime)                      // Set created-by volsync label

		logger.Info("Creating temp populator pvc from snapshot", "volpop pvc name", pvcPrime.GetName())
		err = r.Create(ctx, pvcPrime)
		if err != nil {
			r.EventRecorder.Eventf(pvc, corev1.EventTypeWarning, volsyncv1alpha1.EvRVolPopPVCCreationError,
				"Failed to create populator PVC: %s", err)
			return nil, &vpResult{ctrl.Result{}, err}
		}

		r.EventRecorder.Eventf(pvc, corev1.EventTypeNormal, volsyncv1alpha1.EvRVolPopPVCCreationSuccess,
			"Populator pvc created from snapshot %s", latestImage.Name)
	}

	return pvcPrime, nil
}

func (r *VolumePopulatorReconciler) rebindPVClaim(ctx context.Context, logger logr.Logger,
	pvc, pvcPrime *corev1.PersistentVolumeClaim) *vpResult {
	// Get PV from pvcPrime
	if pvcPrime.Spec.VolumeName == "" {
		// No volume yet
		return &vpResult{ctrl.Result{}, nil}
	}
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvcPrime.Spec.VolumeName,
		},
	}
	err := r.Get(ctx, client.ObjectKeyFromObject(pv), pv)
	if err != nil {
		if !kerrors.IsNotFound(err) {
			return &vpResult{ctrl.Result{}, err}
		}
		// We'll get called again later when the PV exists
		// Should get reconciled on pvcPrime being updated when the PV binds
		return &vpResult{ctrl.Result{}, nil}
	}

	// Examine the claimref for the PV and see if it's bound to the correct PVC
	claimRef := pv.Spec.ClaimRef
	if claimRef == nil ||
		claimRef.Name != pvc.Name ||
		claimRef.Namespace != pvc.Namespace ||
		claimRef.UID != pvc.UID {
		// Make new PV with strategic patch values to perform the PV rebind
		patchPv := &corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name: pv.Name,
				Annotations: map[string]string{
					annotationPopulatedFrom: pvc.Namespace + "/" + pvcPrime.Spec.DataSourceRef.Name,
				},
			},
			Spec: corev1.PersistentVolumeSpec{
				ClaimRef: &corev1.ObjectReference{
					Namespace:       pvc.Namespace,
					Name:            pvc.Name,
					UID:             pvc.UID,
					ResourceVersion: pvc.ResourceVersion,
				},
			},
		}
		patchData, err := json.Marshal(patchPv)
		if err != nil {
			return &vpResult{ctrl.Result{}, err}
		}
		logger.Info("Patching PV claim", "pv name", pv.Name)
		err = r.Patch(ctx, pv, client.RawPatch(types.StrategicMergePatchType, patchData))
		if err != nil {
			return &vpResult{ctrl.Result{}, err}
		}

		// Don't start cleaning up yet -- we need the bind controller to acknowledge the switch
		return &vpResult{ctrl.Result{}, nil}
	}

	return nil // No claimRef yet on PV or PV has already been bound to pvc
}

func (r *VolumePopulatorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("volsync-volume-populator").
		For(&corev1.PersistentVolumeClaim{}, builder.WithPredicates(pvcForVolumePopulatorFilterPredicate())).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 100,
		}).
		Owns(&corev1.PersistentVolumeClaim{}, builder.WithPredicates(pvcOwnedByPredicate())).
		Watches(&volsyncv1alpha1.ReplicationDestination{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, o client.Object) []reconcile.Request {
				return mapFuncReplicationDestinationToVolumePopulatorPVC(ctx, mgr.GetClient(), o)
			}), builder.WithPredicates(replicationDestinationPredicate())).
		Watches(&storagev1.StorageClass{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, o client.Object) []reconcile.Request {
				return mapFuncStorageClassToVolumePopulatorPVC(ctx, mgr.GetClient(), o)
			}), builder.WithPredicates(storageClassPredicate())).
		Complete(r)
}

// Predicate for PVCs with owner (and controller=true) of a PVC - this is to reconcile our temp populator pvc
// (i.e. pvcPrime).  In case there are other PVCs owned by a PVC, predicate will check for our labelPvcPrime to filter
// those out.
func pvcOwnedByPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return utils.HasLabel(e.Object, labelPvcPrime)
		},
		DeleteFunc: func(_ event.DeleteEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return utils.HasLabel(e.ObjectNew, labelPvcPrime)
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return utils.HasLabel(e.Object, labelPvcPrime)
		},
	}
}

func pvcForVolumePopulatorFilterPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			pvc, ok := e.Object.(*corev1.PersistentVolumeClaim)
			if !ok {
				return false
			}
			return pvcHasReplicationDestinationDataSourceRef(pvc)
		},
		DeleteFunc: func(_ event.DeleteEvent) bool {
			// Do not reconcile on PVC deletes
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			pvc, ok := e.ObjectNew.(*corev1.PersistentVolumeClaim)
			if !ok {
				return false
			}
			return pvcHasReplicationDestinationDataSourceRef(pvc)
		},
		GenericFunc: func(e event.GenericEvent) bool {
			pvc, ok := e.Object.(*corev1.PersistentVolumeClaim)
			if !ok {
				return false
			}
			return pvcHasReplicationDestinationDataSourceRef(pvc)
		},
	}
}

func replicationDestinationPredicate() predicate.Predicate {
	// Only reconcile pvcs for replication destination if replication destination is new or updated (no delete)
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

func storageClassPredicate() predicate.Predicate {
	// Only reconcile pvcs for storageclass on storageclass creation
	return predicate.Funcs{
		CreateFunc: func(_ event.CreateEvent) bool {
			return true
		},
		DeleteFunc: func(_ event.DeleteEvent) bool {
			return false
		},
		UpdateFunc: func(_ event.UpdateEvent) bool {
			return false
		},
		GenericFunc: func(_ event.GenericEvent) bool {
			return false
		},
	}
}

func mapFuncReplicationDestinationToVolumePopulatorPVC(ctx context.Context, k8sClient client.Client,
	o client.Object) []reconcile.Request {
	logger := ctrl.Log.WithName("mapFuncReplicationDestinationToVolumePopulatorPVC")

	replicationDestination, ok := o.(*volsyncv1alpha1.ReplicationDestination)
	if !ok {
		return []reconcile.Request{}
	}

	// Find PVCs that use this ReplicationDestination in their dataSourceRef (using index)
	pvcList := &corev1.PersistentVolumeClaimList{}
	err := k8sClient.List(ctx, pvcList,
		client.MatchingFields{
			VolPopPVCToReplicationDestinationIndex: replicationDestination.GetName()}, // custom index
		client.InNamespace(replicationDestination.GetNamespace()))
	if err != nil {
		logger.Error(err, "Error looking up pvcs (using index) matching replication destination",
			"rd name", replicationDestination.GetName(), "namespace", replicationDestination.GetNamespace(),
			"index name", VolPopPVCToReplicationDestinationIndex)
		return []reconcile.Request{}
	}

	// Only enqueue a reconcile request if our PVC for volume populator is not already bound
	return filterRequestsOnlyUnboundPVCs(pvcList)
}

func mapFuncStorageClassToVolumePopulatorPVC(ctx context.Context, k8sClient client.Client,
	o client.Object) []reconcile.Request {
	logger := ctrl.Log.WithName("mapFuncStorageClassToVolumePopulatorPVC")

	storageClass, ok := o.(*storagev1.StorageClass)
	if !ok {
		return []reconcile.Request{}
	}

	// Find PVCs that have this storageClassName set in their spec (using index)
	// Our custom index is only storing PVCs that have a dataSourceRef pointing to a ReplicationDestination
	pvcList := &corev1.PersistentVolumeClaimList{}
	err := k8sClient.List(ctx, pvcList,
		client.MatchingFields{
			VolPopPVCToStorageClassIndex: storageClass.GetName()}, // custom index
	)
	if err != nil {
		logger.Error(err, "Error looking up pvcs for the VolSync volume populator (using index) matching storageclass",
			"storageclass name", storageClass.GetName(), "index name", VolPopPVCToStorageClassIndex)
		return []reconcile.Request{}
	}

	// Only enqueue a reconcile request if our PVC for volume populator is not already bound
	return filterRequestsOnlyUnboundPVCs(pvcList)
}

func filterRequestsOnlyUnboundPVCs(pvcList *corev1.PersistentVolumeClaimList) []reconcile.Request {
	reqs := []reconcile.Request{}

	for i := range pvcList.Items {
		pvc := pvcList.Items[i]
		// Only reconcile pvcs for an RD if the pvc is not already bound to a volume
		if !isPVCBoundToVolume(&pvc) {
			reqs = append(reqs, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      pvc.GetName(),
					Namespace: pvc.GetNamespace(),
				},
			})
		}
	}
	return reqs
}

func isPVCBoundToVolume(pvc *corev1.PersistentVolumeClaim) bool {
	// If pvc.Spec.VolumeName is set, PVC is bound to a volume already
	return pvc.Spec.VolumeName != ""
}

func (r VolumePopulatorReconciler) getReplicationDestinationFromDataSourceRef(ctx context.Context, logger logr.Logger,
	pvc *corev1.PersistentVolumeClaim) (*volsyncv1alpha1.ReplicationDestination, error) {
	// dataSourceRef should be pointing to a ReplicationDestination (see predicates)
	rdName := pvc.Spec.DataSourceRef.Name
	rdNamespace := pvc.GetNamespace()
	// Future, if we allow cross-namespace:
	// if pvc.Spec.DataSourceRef.Namespace != nil && *pvc.Spec.DataSourceRef.Namespace != "" {
	//		rdNamespace = *pvc.Spec.DataSourceRef.Namespace
	//	}
	replicationDestinationForVolPop := &volsyncv1alpha1.ReplicationDestination{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rdName,
			Namespace: rdNamespace,
		},
	}
	err := r.Get(ctx,
		client.ObjectKeyFromObject(replicationDestinationForVolPop), replicationDestinationForVolPop)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Error(err, "Unable to populate volume - replicationdestination not found",
				"name", rdName, "namespace", rdNamespace)
		}
		return nil, err
	}

	return replicationDestinationForVolPop, nil
}

// Cleanup
//   - if any snapshots have our vol pop label for our PVC, remove the label
//   - if do-not-delete label is on the snapshot, remove our ownerref so we will not cause GC of the snap to happen
//   - if pvcPrime is not nil, we will assume it exists and needs to be cleaned up
func (r *VolumePopulatorReconciler) cleanup(ctx context.Context, logger logr.Logger,
	pvc, pvcPrime *corev1.PersistentVolumeClaim) error {
	snapsForPVC, err := r.listSnapshotsUsedByVolPopForPVC(ctx, pvc)
	if err != nil {
		return err
	}
	for i := range snapsForPVC {
		snap := snapsForPVC[i]
		// Remove our vol pop label
		updated := utils.RemoveLabel(&snap, getSnapshotInUseLabelKey(pvc))

		// Check if do-not-delete label was added by anyone else and remove our ownerRef if so
		if utils.IsMarkedDoNotDelete(&snap) && pvcPrime != nil {
			updated = utils.RemoveOwnerReference(&snap, pvcPrime) || updated
		}

		if updated {
			if err := r.Update(ctx, &snap); err != nil {
				logger.Error(err, "Failed to update labels on snapshot")
				return err
			}
		}
	}

	// If PVC' still exists, delete it
	if pvcPrime != nil && pvcPrime.GetDeletionTimestamp().IsZero() {
		logger.Info("Cleanup - deleting temp volume populator PVC", "volpop pvc name", pvcPrime.GetName())
		if err := r.Delete(ctx, pvcPrime); err != nil {
			return err
		}
	}

	return nil
}

// Validates snapshot exists and adds a label specific to our pvc
func (r *VolumePopulatorReconciler) validateSnapshotAndLabel(ctx context.Context, logger logr.Logger,
	snapshotName, namespace string, pvc *corev1.PersistentVolumeClaim,
) (*snapv1.VolumeSnapshot, error) {
	logger = logger.WithValues("snapshot name", snapshotName, "namespace", namespace)

	snapshot := &snapv1.VolumeSnapshot{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      snapshotName,
		Namespace: namespace,
	}, snapshot)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Error(err, "VolumeSnapshot not found")
		}
		return nil, err
	}

	snapInUseLabelKey := getSnapshotInUseLabelKey(pvc)
	snapInUseLabelVal := pvc.GetName()

	// Add label linking to the original PVC for this volume populator
	updated := utils.AddLabel(snapshot, snapInUseLabelKey, snapInUseLabelVal)

	if updated {
		if err := r.Update(ctx, snapshot); err != nil {
			logger.Error(err, "Failed to label snapshot")
			return nil, err
		}
	}

	return snapshot, nil
}

func (r *VolumePopulatorReconciler) ensureOwnerReferenceOnSnapshots(ctx context.Context,
	pvc, pvcPrime *corev1.PersistentVolumeClaim) error {
	// Make sure all the snapshots we previously labeled have owner ref so they can be garbage collected
	// if necessary when pvcPrime is removed
	// Could possibly have multiple snapshots here if we previously marked one as do-not-delete but were not
	// able to create pvcPrime pointing to it (i.e. ReplicationDestination.status.latestImage was updated in-between)
	snapshots, err := r.listSnapshotsUsedByVolPopForPVC(ctx, pvc)
	if err != nil {
		return err
	}
	for i := range snapshots {
		snapshot := snapshots[i]
		// Make sure the snapshot is owned by pvcPrime to prevent others (pvcs w/ volume populator using the same
		// snapshot or replicationdestination) from removing it
		// Ownership is just there for cleanup later on - if marked do-not-delete then no need for ownership
		if !utils.IsMarkedDoNotDelete(&snapshot) {
			err = r.ensureOwnerReferenceOnSnapshot(ctx, &snapshot, pvcPrime)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *VolumePopulatorReconciler) listSnapshotsUsedByVolPopForPVC(ctx context.Context,
	pvc *corev1.PersistentVolumeClaim) ([]snapv1.VolumeSnapshot, error) {
	snapInUseLabelKey := getSnapshotInUseLabelKey(pvc)

	// Find all snapshots in the namespace with our volume populator label corresponding to this pvc
	ls, err := labels.Parse(snapInUseLabelKey)
	if err != nil {
		return nil, err
	}

	listOptions := []client.ListOption{
		client.MatchingLabelsSelector{
			Selector: ls,
		},
		client.InNamespace(pvc.GetNamespace()),
	}
	snapList := &snapv1.VolumeSnapshotList{}
	err = r.List(ctx, snapList, listOptions...)
	if err != nil {
		return nil, err
	}

	return snapList.Items, nil
}

func (r *VolumePopulatorReconciler) ensureOwnerReferenceOnSnapshot(ctx context.Context, snapshot *snapv1.VolumeSnapshot,
	owner metav1.Object) error {
	updated, err := r.addOwnerReference(snapshot, owner)
	if err != nil {
		return err
	}
	if updated {
		return r.Update(ctx, snapshot)
	}
	// No update required
	return nil
}

func (r *VolumePopulatorReconciler) addOwnerReference(obj, owner metav1.Object) (bool, error) {
	currentOwnerRefs := obj.GetOwnerReferences()

	err := ctrlutil.SetOwnerReference(owner, obj, r.Client.Scheme())
	if err != nil {
		return false, fmt.Errorf("%w", err)
	}

	updated := !reflect.DeepEqual(obj.GetOwnerReferences(), currentOwnerRefs)

	return updated, nil
}

func getPVCPrimeName(pvc *corev1.PersistentVolumeClaim) string {
	return fmt.Sprintf("%s-%s", populatorPvcPrefix, pvc.UID)
}

// Finds PVCPrime - will return nil if PVCPrime is not found
func GetVolumePopulatorPVCPrime(ctx context.Context, c client.Client,
	pvc *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	pvcPrime := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getPVCPrimeName(pvc),
			Namespace: pvc.GetNamespace(),
		},
	}
	err := c.Get(ctx, client.ObjectKeyFromObject(pvcPrime), pvcPrime)
	if err != nil {
		if kerrors.IsNotFound(err) {
			return nil, nil // Return nil if not found, no error
		}
		return nil, err
	}
	return pvcPrime, nil
}

func getSnapshotInUseLabelKey(pvc *corev1.PersistentVolumeClaim) string {
	return fmt.Sprintf("%s%s", utils.SnapInUseByVolumePopulatorLabelPrefix, pvc.UID)
}

func pvcHasReplicationDestinationDataSourceRef(pvc *corev1.PersistentVolumeClaim) bool {
	if pvc.Spec.DataSourceRef == nil || pvc.Spec.DataSourceRef.APIGroup == nil {
		return false
	}

	// This volume populator responds to PVCs with dataSourceRef with group==volsync.backube
	// and kind==ReplicationDestination
	return *pvc.Spec.DataSourceRef.APIGroup == volsyncv1alpha1.GroupVersion.Group &&
		pvc.Spec.DataSourceRef.Kind == "ReplicationDestination" &&
		pvc.Spec.DataSourceRef.Name != ""
}

func checkIntreeStorageClass(pvc *corev1.PersistentVolumeClaim, sc *storagev1.StorageClass) error {
	if !strings.HasPrefix(sc.Provisioner, "kubernetes.io/") {
		// This is not an in-tree StorageClass
		return nil
	}

	if pvc.Annotations != nil {
		if migrated := pvc.Annotations[volume.AnnMigratedTo]; migrated != "" {
			// The PVC is migrated to CSI
			return nil
		}
	}

	// The SC is in-tree & PVC is not migrated
	return fmt.Errorf("in-tree volume volume plugin %q cannot use volume populator", sc.Provisioner)
}
