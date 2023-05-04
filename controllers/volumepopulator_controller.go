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

package controllers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/controllers/utils"
)

const (
	populatorPvcPrefix      string = "vs-prime"
	annotationSelectedNode  string = "volume.kubernetes.io/selected-node"
	annotationPopulatedFrom string = "volsync.backube/populated-from"
	labelPvcPrime           string = utils.VolsyncLabelPrefix + "/populator-pvc-for"

	reasonPVCPopulatorFinished = "VolSyncPopulatorFinished"
	reasonPVCPopulatorError    = "VolSyncPopulatorError"
	reasonPVCCreationSuccess   = "VolSyncPopulatorPVCCreated"
	reasonPVCCreationError     = "VolSyncPopulatorPVCCreationError"
)

//nolint:lll
//+kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=persistentvolumes,verbs=get;list;watch;patch
//+kubebuilder:rbac:groups=volsync.backube,resources=replicationdestinations,verbs=get;list;watch
//+kubebuilder:rbac:groups=storage.k8s.io,resources=storageclasses,verbs=get;list;watch

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

// Reconcile logic is adapted from reference at:
// https://github.com/kubernetes-csi/lib-volume-populator/blob/master/populator-machinery/controller.go
//
//nolint:funlen
func (r *VolumePopulatorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("pvc", req.NamespacedName)

	logger.Info("Reconciling ...") //TODO: remove

	// Get PVC CR instance
	pvc := &corev1.PersistentVolumeClaim{}
	if err := r.Client.Get(ctx, req.NamespacedName, pvc); err != nil {
		if !kerrors.IsNotFound(err) {
			logger.Error(err, "Failed to get PVC")
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check to make sure we should be reconciling this PVC - just in case
	if shouldReconcile := pvcHasReplicationDestinationDataSourceRef(pvc); !shouldReconcile {
		return ctrl.Result{}, nil
	}

	var waitForFirstConsumer bool
	var nodeName string
	if pvc.Spec.StorageClassName != nil {
		storageClassName := *pvc.Spec.StorageClassName

		storageClass := &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: storageClassName,
			},
		}
		err := r.Client.Get(ctx, client.ObjectKeyFromObject(storageClass), storageClass)
		if err != nil {
			if !errors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
			//c.addNotification(key, "sc", "", storageClassName)
			//TODO: no requeue will happen unless we also watch storageclasses
			// We'll get called again later when the storage class exists
			return ctrl.Result{}, nil
		}

		if err := checkIntreeStorageClass(pvc, storageClass); err != nil {
			logger.Info("Ignoring PVC")
			return ctrl.Result{}, nil
		}

		if storageClass.VolumeBindingMode != nil &&
			storagev1.VolumeBindingWaitForFirstConsumer == *storageClass.VolumeBindingMode {
			waitForFirstConsumer = true
			nodeName = pvc.Annotations[annotationSelectedNode]
			if nodeName == "" {
				// Wait for the PVC to get a node name before continuing
				logger.Info("VolumeBindingMode is WaitForFirstConsumer, need to wait for nodeName annotation",
					"annotation name", annotationSelectedNode)
				return ctrl.Result{}, nil
			}
		}
	}

	//TODO: what if no StorageClassName in the pvc.spec?

	// Look for PVC' - this will be a PVC with the dataSourceRef set to the latest snapshot image
	// from the ReplicationDestination
	pvcPrimeName := fmt.Sprintf("%s-%s", populatorPvcPrefix, pvc.UID)
	//c.addNotification(key, "pvc", c.populatorNamespace, pvcPrimeName)
	pvcPrime := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcPrimeName,
			Namespace: pvc.GetNamespace(),
		},
	}
	err := r.Client.Get(ctx, client.ObjectKeyFromObject(pvcPrime), pvcPrime)
	if err != nil {
		if !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		pvcPrime = nil // set to nil if no pvcPrime found yet
	}

	// If the PVC is unbound, we need to perform the population
	if "" == pvc.Spec.VolumeName {
		/*
			// Ensure the PVC has a finalizer on it so we can clean up the stuff we create
			err = c.ensureFinalizer(ctx, pvc, c.pvcFinalizer, true)
			if err != nil {
				return err
			}

			// Record start time for populator metric
			c.metrics.operationStart(pvc.UID)
		*/

		if pvcPrime == nil {
			// pvcPrime doesn't exist yet
			// Check for existence of ReplicationDestination here - if PVC' was already there, then it may
			// be ok if replicationdestination is missing - so only error out here if RD doesn't exist
			rd, err := r.getReplicationDestinationFromDataSourceRef(ctx, logger, pvc)
			if err != nil {
				return ctrl.Result{}, err
			}

			logger = logger.WithValues("replication destination name", rd.GetName(), "namespace", rd.GetNamespace())

			if rd.Status == nil || rd.Status.LatestImage == nil {
				logger.Info("ReplicationDestination has no latestImage, cannot populate volume yet")
				//TODO: should we requeue here? Ideally we'd want to wait for updates on the rd
				return ctrl.Result{}, nil
			}

			latestImage := rd.Status.LatestImage

			if !utils.IsSnapshot(latestImage) {
				// This means the replicationdestination is using "Direct" (aka "None") CopyMethod
				dataSourceRefErr := fmt.Errorf("ReplicationDestination latestImage is not a volumesnapshot")
				logger.Error(dataSourceRefErr, "Unable to populate volume")
				r.EventRecorder.Eventf(pvc, corev1.EventTypeWarning, reasonPVCPopulatorError,
					"Unable to populate volume: %s", dataSourceRefErr)
				// Do not return error here - no use retrying
				return ctrl.Result{}, nil
			}

			_, err = r.validateSnapshotAndMarkDoNotDelete(ctx, logger, latestImage.Name, rd.GetNamespace())
			if err != nil {
				return ctrl.Result{}, err
			}

			pvcPrime = &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvcPrimeName,
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
			if err := ctrl.SetControllerReference(pvc, pvcPrime, r.Client.Scheme()); err != nil {
				logger.Error(err, utils.ErrUnableToSetControllerRef)
				return ctrl.Result{}, err
			}
			utils.AddLabel(pvcPrime, labelPvcPrime, pvc.GetName()) // Use this filter in predicates in the &Owns() watcher
			utils.SetOwnedByVolSync(pvcPrime)                      // Set created-by volsync label

			//TODO: update this to ctrlutil.CreateOrUpdate (or possibly use our own CreateOrUpdateDeleteOnImmutableErr)
			logger.Info("Creating temp populator pvc from snapshot", "volpop pvc name", pvcPrime.GetName())
			err = r.Client.Create(ctx, pvcPrime)
			if err != nil {
				r.EventRecorder.Eventf(pvc, corev1.EventTypeWarning, reasonPVCCreationError,
					"Failed to create populator PVC: %s", err)
				return ctrl.Result{}, err
			}

			r.EventRecorder.Eventf(pvc, corev1.EventTypeNormal, reasonPVCCreationSuccess,
				"Populator pvc created from snapshot %s", latestImage.Name)
		}

		// Get PV from pvcPrime
		if pvcPrime.Spec.VolumeName == "" {
			// No volume yet
			logger.Info("temp volume populator pvc has no PV yet", "volpop pvc name", pvcPrime.GetName())
			return ctrl.Result{}, nil
		}
		pv := &corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name: pvcPrime.Spec.VolumeName,
			},
		}
		//c.addNotification(key, "pv", "", pvcPrime.Spec.VolumeName)
		err = r.Client.Get(ctx, client.ObjectKeyFromObject(pv), pv)
		if err != nil {
			if !errors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
			// We'll get called again later when the PV exists
			return ctrl.Result{}, nil
		}

		// Examine the claimref for the PV and see if it's bound to the correct PVC
		claimRef := pv.Spec.ClaimRef
		if claimRef.Name != pvc.Name || claimRef.Namespace != pvc.Namespace || claimRef.UID != pvc.UID {
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
			var patchData []byte
			patchData, err = json.Marshal(patchPv)
			if err != nil {
				return ctrl.Result{}, err
			}
			logger.Info("Patching PV claim", "pv name", pv.Name)
			err = r.Client.Patch(ctx, pv, client.RawPatch(types.StrategicMergePatchType, patchData))
			if err != nil {
				logger.Error(err, "error patching PV claim")
				return ctrl.Result{}, err
			}

			// Don't start cleaning up yet -- we need the bind controller to acknowledge the switch
			return ctrl.Result{}, nil
		}
	}

	// Wait for the bind controller to rebind the PV
	if pvcPrime != nil {
		if corev1.ClaimLost != pvcPrime.Status.Phase {
			logger.Info("Waiting for pv rebind", "from pvc", pvcPrime.GetName(), "to pvc", pvc.GetName())
			return ctrl.Result{}, nil
		}
	}

	/*
		// Record start time for populator metric
		c.metrics.recordMetrics(pvc.UID, "success")
	*/

	// *** At this point the volume population is done and we're just cleaning up ***
	r.EventRecorder.Eventf(pvc, corev1.EventTypeNormal, reasonPVCPopulatorFinished, "Populator finished")

	// Cleanup
	// If PVC' still exists, delete it
	if pvcPrime != nil && pvcPrime.GetDeletionTimestamp().IsZero() {
		logger.Info("Cleanup - deleting temp volume populator PVC", "volpop pvc name", pvcPrime.GetName())
		if err := r.Client.Delete(ctx, pvcPrime); err != nil {
			return ctrl.Result{}, err
		}
	}

	/*
		// Make sure the PVC finalizer is gone
		err = c.ensureFinalizer(ctx, pvc, c.pvcFinalizer, false)
		if err != nil {
			return err
		}

		// Clean up our internal callback maps
		c.cleanupNotifications(key)
	*/

	return ctrl.Result{}, nil
}

func (r *VolumePopulatorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		//(using Watches() below instead) For(&corev1.PersistentVolumeClaim{}).
		Named("volsync-volume-populator").
		For(&corev1.PersistentVolumeClaim{}, builder.WithPredicates(pvcForVolumePopulatorFilterPredicate())).
		WithOptions(controller.Options{
			//MaxConcurrentReconciles: 100,
			MaxConcurrentReconciles: 1,
		}).
		Owns(&corev1.PersistentVolumeClaim{}, builder.WithPredicates(pvcOwnedByPredicate())).
		/*
			Watches(&source.Kind{Type: &volsyncv1alpha1.ReplicationDestination{}},
				&handler.EnqueueRequestForOwner{OwnerType: &corev1.PersistentVolumeClaim{}, IsController: false}).
		*/
		Complete(r)
}

// Predicate for PVCs with owner (and controller=true) of a PVC - this is to reconcile our temp populator pvc
// (i.e. pvcPrime).  In case there are other PVCs owned by PVC, predicate will check for our labelPvcPrime to filter
// those out.
func pvcOwnedByPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return utils.HasLabel(e.Object, labelPvcPrime)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
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
		DeleteFunc: func(e event.DeleteEvent) bool {
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
	err := r.Client.Get(ctx,
		client.ObjectKeyFromObject(replicationDestinationForVolPop), replicationDestinationForVolPop)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Error(err, "Unable to populate volume - replicationdestination not found",
				"name", rdName, "namespace", rdNamespace)
		}
		return nil, err
	}

	return replicationDestinationForVolPop, nil
}

// Validates snapshot exists and adds VolSync "do-not-delete" label to indicate the snapshot should not be cleaned up
func (r *VolumePopulatorReconciler) validateSnapshotAndMarkDoNotDelete(ctx context.Context, logger logr.Logger,
	snapshotName, namespace string,
) (*snapv1.VolumeSnapshot, error) {
	logger = logger.WithValues("snapshot name", snapshotName, "namespace", namespace)

	snapshot := &snapv1.VolumeSnapshot{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      snapshotName,
		Namespace: namespace,
	}, snapshot)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Error(err, "VolumeSnapshot not found")
		}
		return nil, err
	}

	// Add label to indicate that VolSync should not delete/cleanup this snapshot
	needsUpdate := utils.MarkDoNotDelete(snapshot)
	if needsUpdate {
		if err := r.Client.Update(ctx, snapshot); err != nil {
			logger.Error(err, "Failed to mark snapshot do not delete")
			return nil, err
		}
		logger.Info("Snapshot marked do-not-delete")
	}

	//TODO: do we also add owner ref or something of the sort? - need to think about cleanup

	return snapshot, nil
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
	//FIXME: determine if this is necessary
	return nil
	/*
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
	*/
}
