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

package volumehandler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/controllers/mover"
	"github.com/backube/volsync/controllers/utils"
)

const (
	// Annotation used to track the name of the snapshot being created
	snapshotAnnotation = "volsync.backube/snapname"
	// Time format for snapshot names and labels
	timeYYYYMMDDHHMMSS = "20060102150405"
)

type VolumeHandler struct {
	client                  client.Client
	eventRecorder           events.EventRecorder
	owner                   client.Object
	copyMethod              volsyncv1alpha1.CopyMethodType
	capacity                *resource.Quantity
	storageClassName        *string
	accessModes             []corev1.PersistentVolumeAccessMode
	volumeMode              corev1.PersistentVolumeMode
	volumeSnapshotClassName *string
}

// EnsurePVCFromSrc ensures the presence of a PVC that is based on the provided
// src PVC. It is generated based on the VolumeHandler's configuration. It may
// be the same PVC as src. Note: it's possible to return nil, nil. In this case,
// the operation should be retried.
func (vh *VolumeHandler) EnsurePVCFromSrc(ctx context.Context, log logr.Logger,
	src *corev1.PersistentVolumeClaim, name string, isTemporary bool) (*corev1.PersistentVolumeClaim, error) {
	// make sure the volumeMode is set properly
	vh.volumeMode = corev1.PersistentVolumeFilesystem
	if src.Spec.VolumeMode != nil {
		vh.volumeMode = *src.Spec.VolumeMode
	}
	switch vh.copyMethod {
	case volsyncv1alpha1.CopyMethodNone:
		fallthrough // Same as CopyMethodDirect
	case volsyncv1alpha1.CopyMethodDirect:
		return src, nil
	case volsyncv1alpha1.CopyMethodClone:
		return vh.ensureClone(ctx, log, src, name, isTemporary)
	case volsyncv1alpha1.CopyMethodSnapshot:
		snap, err := vh.ensureSnapshot(ctx, log, src, name, isTemporary)
		if snap == nil || err != nil {
			return nil, err
		}
		return vh.pvcFromSnapshot(ctx, log, snap, src, name, isTemporary)
	default:
		return nil, fmt.Errorf("unsupported copyMethod: %v -- must be Direct, None, Clone, or Snapshot", vh.copyMethod)
	}
}

// EnsureImage ensures the presence of a representation of the provided src
// PVC. It is generated based on the VolumeHandler's configuration and could be
// of type PersistentVolumeClaim or VolumeSnapshot. It may even be the same PVC
// as src.
func (vh *VolumeHandler) EnsureImage(ctx context.Context, log logr.Logger,
	src *corev1.PersistentVolumeClaim) (*corev1.TypedLocalObjectReference, error) {
	switch vh.copyMethod { //nolint: exhaustive
	case volsyncv1alpha1.CopyMethodNone:
		fallthrough // Same as CopyMethodDirect
	case volsyncv1alpha1.CopyMethodDirect:
		return &corev1.TypedLocalObjectReference{
			APIGroup: &corev1.SchemeGroupVersion.Group,
			Kind:     src.Kind,
			Name:     src.Name,
		}, nil
	case volsyncv1alpha1.CopyMethodSnapshot:
		snap, err := vh.ensureImageSnapshot(ctx, log, src)
		if snap == nil || err != nil {
			return nil, err
		}

		snapKind := snap.Kind
		if snapKind == "" {
			// In case kind is not filled out, although it should be when read from k8sclient cache
			// Unit tests that use a direct API client may not have kind - but we can get it from the scheme
			gvks, _, _ := vh.client.Scheme().ObjectKinds(snap)
			for _, gvk := range gvks {
				if gvk.Kind != "" {
					snapKind = gvk.Kind
					break
				}
			}
		}

		return &corev1.TypedLocalObjectReference{
			APIGroup: &snapv1.SchemeGroupVersion.Group,
			Kind:     snapKind,
			Name:     snap.Name,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported copyMethod: %v -- must be Direct, None, or Snapshot", vh.copyMethod)
	}
}

func (vh *VolumeHandler) UseProvidedPVC(ctx context.Context, pvcName string) (*corev1.PersistentVolumeClaim, error) {
	return vh.getPVCByName(ctx, pvcName)
}

func (vh *VolumeHandler) getPVCByName(ctx context.Context, pvcName string) (*corev1.PersistentVolumeClaim, error) {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: vh.owner.GetNamespace(),
		},
	}
	err := vh.client.Get(ctx, client.ObjectKeyFromObject(pvc), pvc)
	return pvc, err
}

// nolint: funlen
func (vh *VolumeHandler) EnsureNewPVC(ctx context.Context, log logr.Logger,
	name string) (*corev1.PersistentVolumeClaim, error) {
	logger := log.WithValues("PVC", name)

	// Ensure required configuration parameters have been provided in order to
	// create volume
	if len(vh.accessModes) == 0 {
		err := errors.New("accessModes must be provided when destinationPVC is not")
		logger.Error(err, "error allocating new PVC")
		return nil, err
	}
	if vh.capacity == nil {
		err := errors.New("capacity must be provided when destinationPVC is not")
		logger.Error(err, "error allocating new PVC")
		return nil, err
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: vh.owner.GetNamespace(),
		},
	}

	op, err := ctrlutil.CreateOrUpdate(ctx, vh.client, pvc, func() error {
		if err := ctrl.SetControllerReference(vh.owner, pvc, vh.client.Scheme()); err != nil {
			logger.Error(err, utils.ErrUnableToSetControllerRef)
			return err
		}
		utils.SetOwnedByVolSync(pvc)
		if pvc.CreationTimestamp.IsZero() { // set immutable fields
			pvc.Spec.AccessModes = vh.accessModes
			pvc.Spec.StorageClassName = vh.storageClassName
			volumeMode := corev1.PersistentVolumeFilesystem
			pvc.Spec.VolumeMode = &volumeMode
		}

		pvc.Spec.Resources.Requests = corev1.ResourceList{
			corev1.ResourceStorage: *vh.capacity,
		}
		return nil
	})

	if err != nil {
		logger.Error(err, "reconcile failed")
		return nil, err
	}
	logger.V(1).Info("PVC reconciled", "operation", op)
	if op == ctrlutil.OperationResultCreated {
		vh.eventRecorder.Eventf(vh.owner, pvc, corev1.EventTypeNormal,
			volsyncv1alpha1.EvRPVCCreated, volsyncv1alpha1.EvACreatePVC,
			"created %s to receive incoming data",
			utils.KindAndName(vh.client.Scheme(), pvc))
	}
	if pvc.Status.Phase != corev1.ClaimBound &&
		!pvc.CreationTimestamp.IsZero() &&
		pvc.CreationTimestamp.Add(mover.PVCBindTimeout).Before(time.Now()) {
		vh.eventRecorder.Eventf(vh.owner, pvc, corev1.EventTypeWarning,
			volsyncv1alpha1.EvRPVCNotBound, "",
			"waiting for %s to bind; check StorageClass name and CSI driver capabilities",
			utils.KindAndName(vh.client.Scheme(), pvc))
	}

	return pvc, nil
}

func (vh *VolumeHandler) SetAccessModes(accessModes []corev1.PersistentVolumeAccessMode) {
	vh.accessModes = accessModes
}

func (vh *VolumeHandler) GetAccessModes() []corev1.PersistentVolumeAccessMode {
	return vh.accessModes
}

// nolint: funlen
func (vh *VolumeHandler) ensureImageSnapshot(ctx context.Context, log logr.Logger,
	src *corev1.PersistentVolumeClaim) (*snapv1.VolumeSnapshot, error) {
	// create & record name (if necessary)
	if src.Annotations == nil {
		src.Annotations = make(map[string]string)
	}
	if _, ok := src.Annotations[snapshotAnnotation]; !ok {
		ts := time.Now().Format(timeYYYYMMDDHHMMSS)
		src.Annotations[snapshotAnnotation] = src.Name + "-" + ts
		if err := vh.client.Update(ctx, src); err != nil {
			log.Error(err, "unable to annotate PVC")
			return nil, err
		}
	}
	snapName := src.Annotations[snapshotAnnotation]

	// ensure the object
	logger := log.WithValues("snapshot", snapName)

	snap := &snapv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      snapName,
			Namespace: src.Namespace,
		},
	}
	op, err := ctrlutil.CreateOrUpdate(ctx, vh.client, snap, func() error {
		if utils.IsMarkedDoNotDelete(snap) {
			// Remove adding ownership and potentially marking for cleanup if do-not-delete label is present
			utils.UnMarkForCleanupAndRemoveOwnership(snap, vh.owner)
		} else {
			if err := ctrl.SetControllerReference(vh.owner, snap, vh.client.Scheme()); err != nil {
				logger.Error(err, utils.ErrUnableToSetControllerRef)
				return err
			}
			utils.SetOwnedByVolSync(snap)
		}
		if snap.CreationTimestamp.IsZero() {
			snap.Spec = snapv1.VolumeSnapshotSpec{
				Source: snapv1.VolumeSnapshotSource{
					PersistentVolumeClaimName: &src.Name,
				},
				VolumeSnapshotClassName: vh.volumeSnapshotClassName,
			}
		}
		return nil
	})
	if err != nil {
		logger.Error(err, "reconcile failed")
		return nil, err
	}
	logger.V(1).Info("Snapshot reconciled", "operation", op)
	if op == ctrlutil.OperationResultCreated {
		vh.eventRecorder.Eventf(vh.owner, snap, corev1.EventTypeNormal,
			volsyncv1alpha1.EvRSnapCreated, volsyncv1alpha1.EvACreateSnap, "created %s from %s",
			utils.KindAndName(vh.client.Scheme(), snap), utils.KindAndName(vh.client.Scheme(), src))
	}

	// We only continue reconciling if the snapshot has been bound & not deleted
	if !snap.DeletionTimestamp.IsZero() {
		return nil, nil
	}
	if snap.Status == nil || snap.Status.BoundVolumeSnapshotContentName == nil {
		if snap.CreationTimestamp.Add(mover.SnapshotBindTimeout).Before(time.Now()) {
			vh.eventRecorder.Eventf(vh.owner, snap, corev1.EventTypeWarning,
				volsyncv1alpha1.EvRSnapNotBound, volsyncv1alpha1.EvANone,
				"waiting for %s to bind; check VolumeSnapshotClass name and ensure CSI driver supports volume snapshots",
				utils.KindAndName(vh.client.Scheme(), snap))
		}
		return nil, nil
	}

	return snap, nil
}

func (vh *VolumeHandler) RemoveSnapshotAnnotationFromPVC(ctx context.Context, log logr.Logger, pvcName string) error {
	pvc, err := vh.getPVCByName(ctx, pvcName)
	if err != nil {
		if kerrors.IsNotFound(err) {
			return nil // PVC no longer exists, nothing to do
		}
		return err
	}

	delete(pvc.Annotations, snapshotAnnotation)
	if err := vh.client.Update(ctx, pvc); err != nil {
		log.Error(err, "unable to remove snapshot annotation from PVC", "pvc", pvc)
		return err
	}
	return nil
}

// nolint: funlen
func (vh *VolumeHandler) ensureClone(ctx context.Context, log logr.Logger,
	src *corev1.PersistentVolumeClaim, name string, isTemporary bool) (*corev1.PersistentVolumeClaim, error) {
	clone := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: vh.owner.GetNamespace(),
		},
	}
	logger := log.WithValues("clone", client.ObjectKeyFromObject(clone))

	op, err := ctrlutil.CreateOrUpdate(ctx, vh.client, clone, func() error {
		if err := ctrl.SetControllerReference(vh.owner, clone, vh.client.Scheme()); err != nil {
			logger.Error(err, utils.ErrUnableToSetControllerRef)
			return err
		}
		utils.SetOwnedByVolSync(clone)
		if isTemporary {
			utils.MarkForCleanup(vh.owner, clone)
		}
		if clone.CreationTimestamp.IsZero() {
			if vh.capacity != nil {
				clone.Spec.Resources.Requests = corev1.ResourceList{
					corev1.ResourceStorage: *vh.capacity,
				}
			} else if src.Status.Capacity != nil && src.Status.Capacity.Storage() != nil {
				// check the src PVC capacity if set
				clone.Spec.Resources.Requests = corev1.ResourceList{
					corev1.ResourceStorage: *src.Status.Capacity.Storage(),
				}
			} else {
				// Fallback to the pvc requested size
				clone.Spec.Resources.Requests = corev1.ResourceList{
					corev1.ResourceStorage: *src.Spec.Resources.Requests.Storage(),
				}
			}
			if vh.storageClassName != nil {
				clone.Spec.StorageClassName = vh.storageClassName
			} else {
				clone.Spec.StorageClassName = src.Spec.StorageClassName
			}
			if vh.accessModes != nil {
				clone.Spec.AccessModes = vh.accessModes
			} else {
				clone.Spec.AccessModes = src.Spec.AccessModes
			}
			clone.Spec.VolumeMode = &vh.volumeMode
			clone.Spec.DataSource = &corev1.TypedLocalObjectReference{
				APIGroup: nil,
				Kind:     "PersistentVolumeClaim",
				Name:     src.Name,
			}
		}
		return nil
	})
	if err != nil {
		logger.Error(err, "reconcile failed")
		return nil, err
	}
	if !clone.DeletionTimestamp.IsZero() {
		logger.V(1).Info("PVC is being deleted-- need to wait")
		return nil, nil
	}
	logger.V(1).Info("clone reconciled", "operation", op)
	if op == ctrlutil.OperationResultCreated {
		vh.eventRecorder.Eventf(vh.owner, clone, corev1.EventTypeNormal,
			volsyncv1alpha1.EvRPVCCreated, volsyncv1alpha1.EvACreatePVC,
			"created %s as a clone of %s",
			utils.KindAndName(vh.client.Scheme(), clone), utils.KindAndName(vh.client.Scheme(), src))
	}
	if !clone.CreationTimestamp.IsZero() &&
		clone.CreationTimestamp.Add(mover.PVCBindTimeout).Before(time.Now()) &&
		clone.Status.Phase != corev1.ClaimBound {
		vh.eventRecorder.Eventf(vh.owner, clone, corev1.EventTypeWarning,
			volsyncv1alpha1.EvRPVCNotBound, "",
			"waiting for %s to bind; check StorageClass name and ensure CSI driver supports volume cloning",
			utils.KindAndName(vh.client.Scheme(), clone))
	}
	return clone, err
}

// nolint: funlen
func (vh *VolumeHandler) ensureSnapshot(ctx context.Context, log logr.Logger,
	src *corev1.PersistentVolumeClaim, name string, isTemporary bool) (*snapv1.VolumeSnapshot, error) {
	snap := &snapv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: vh.owner.GetNamespace(),
		},
	}
	logger := log.WithValues("snapshot", client.ObjectKeyFromObject(snap))

	op, err := ctrlutil.CreateOrUpdate(ctx, vh.client, snap, func() error {
		if err := ctrl.SetControllerReference(vh.owner, snap, vh.client.Scheme()); err != nil {
			logger.Error(err, utils.ErrUnableToSetControllerRef)
			return err
		}
		utils.SetOwnedByVolSync(snap)
		if isTemporary {
			utils.MarkForCleanup(vh.owner, snap)
		}
		if snap.CreationTimestamp.IsZero() {
			snap.Spec.Source.PersistentVolumeClaimName = &src.Name
			snap.Spec.VolumeSnapshotClassName = vh.volumeSnapshotClassName
		}
		return nil
	})
	if err != nil {
		logger.Error(err, "reconcile failed")
		return nil, err
	}
	if !snap.DeletionTimestamp.IsZero() {
		logger.V(1).Info("snap is being deleted-- need to wait")
		return nil, nil
	}
	if op == ctrlutil.OperationResultCreated {
		vh.eventRecorder.Eventf(vh.owner, snap, corev1.EventTypeNormal,
			volsyncv1alpha1.EvRSnapCreated, volsyncv1alpha1.EvACreateSnap,
			"created %s from %s",
			utils.KindAndName(vh.client.Scheme(), snap), utils.KindAndName(vh.client.Scheme(), src))
	}
	if snap.Status == nil || snap.Status.BoundVolumeSnapshotContentName == nil {
		logger.V(1).Info("waiting for snapshot to be bound")
		if snap.CreationTimestamp.Add(mover.SnapshotBindTimeout).Before(time.Now()) {
			vh.eventRecorder.Eventf(vh.owner, snap, corev1.EventTypeWarning,
				volsyncv1alpha1.EvRSnapNotBound, volsyncv1alpha1.EvANone,
				"waiting for %s to bind; check VolumeSnapshotClass name and ensure CSI driver supports volume snapshots",
				utils.KindAndName(vh.client.Scheme(), snap))
		}
		return nil, nil
	}
	if snap.Status.ReadyToUse != nil && !*snap.Status.ReadyToUse {
		// readyToUse is set to false for this volume snapshot
		logger.V(1).Info("waiting for snapshot to be ready")
		return nil, nil
	}
	// status.readyToUse either is not set by the driver at this point (even though
	// status.BoundVolumeSnapshotContentName is set), or readyToUse=true

	logger.V(1).Info("temporary snapshot reconciled", "operation", op)
	return snap, nil
}

// nolint: funlen
func (vh *VolumeHandler) pvcFromSnapshot(ctx context.Context, log logr.Logger,
	snap *snapv1.VolumeSnapshot, original *corev1.PersistentVolumeClaim,
	name string, isTemporary bool) (*corev1.PersistentVolumeClaim, error) {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: vh.owner.GetNamespace(),
		},
	}
	logger := log.WithValues("pvc", client.ObjectKeyFromObject(pvc))

	op, err := ctrlutil.CreateOrUpdate(ctx, vh.client, pvc, func() error {
		if err := ctrl.SetControllerReference(vh.owner, pvc, vh.client.Scheme()); err != nil {
			logger.Error(err, utils.ErrUnableToSetControllerRef)
			return err
		}
		utils.SetOwnedByVolSync(pvc)
		if isTemporary {
			utils.MarkForCleanup(vh.owner, pvc)
		}
		if pvc.CreationTimestamp.IsZero() {
			if vh.capacity != nil {
				pvc.Spec.Resources.Requests = corev1.ResourceList{
					corev1.ResourceStorage: *vh.capacity,
				}
			} else if snap.Status != nil && snap.Status.RestoreSize != nil && !snap.Status.RestoreSize.IsZero() {
				pvc.Spec.Resources.Requests = corev1.ResourceList{
					corev1.ResourceStorage: *snap.Status.RestoreSize,
				}
			} else if original.Status.Capacity != nil && original.Status.Capacity.Storage() != nil {
				// check the original PVC capacity if set
				pvc.Spec.Resources.Requests = corev1.ResourceList{
					corev1.ResourceStorage: *original.Status.Capacity.Storage(),
				}
			} else {
				// Fallback to the pvc requested size
				pvc.Spec.Resources.Requests = corev1.ResourceList{
					corev1.ResourceStorage: *original.Spec.Resources.Requests.Storage(),
				}
			}
			if vh.storageClassName != nil {
				pvc.Spec.StorageClassName = vh.storageClassName
			} else {
				pvc.Spec.StorageClassName = original.Spec.StorageClassName
			}
			if vh.accessModes != nil {
				pvc.Spec.AccessModes = vh.accessModes
			} else {
				pvc.Spec.AccessModes = original.Spec.AccessModes
			}
			pvc.Spec.VolumeMode = &vh.volumeMode
			pvc.Spec.DataSource = &corev1.TypedLocalObjectReference{
				APIGroup: &snapv1.SchemeGroupVersion.Group,
				Kind:     "VolumeSnapshot",
				Name:     snap.Name,
			}
		}
		return nil
	})
	if err != nil {
		logger.Error(err, "reconcile failed")
		return nil, err
	}
	if op == ctrlutil.OperationResultCreated {
		vh.eventRecorder.Eventf(vh.owner, pvc, corev1.EventTypeNormal,
			volsyncv1alpha1.EvRPVCCreated, volsyncv1alpha1.EvACreatePVC, "created %s from %s",
			utils.KindAndName(vh.client.Scheme(), pvc), utils.KindAndName(vh.client.Scheme(), snap))
	}
	if pvc.Status.Phase != corev1.ClaimBound &&
		!pvc.CreationTimestamp.IsZero() &&
		pvc.CreationTimestamp.Add(mover.PVCBindTimeout).Before(time.Now()) {
		vh.eventRecorder.Eventf(vh.owner, pvc, corev1.EventTypeWarning,
			volsyncv1alpha1.EvRPVCNotBound, "",
			"waiting for %s to bind; check StorageClass name and CSI driver capabilities",
			utils.KindAndName(vh.client.Scheme(), pvc))
	}

	logger.V(1).Info("pvc from snap reconciled", "operation", op)
	return pvc, nil
}

func (vh *VolumeHandler) IsCopyMethodDirect() bool {
	return vh.copyMethod == volsyncv1alpha1.CopyMethodDirect ||
		vh.copyMethod == volsyncv1alpha1.CopyMethodNone
}
