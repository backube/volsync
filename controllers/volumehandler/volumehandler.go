/*
Copyright 2021 The Scribe authors.

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
	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1beta1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	scribev1alpha1 "github.com/backube/scribe/api/v1alpha1"
	"github.com/backube/scribe/controllers/utils"
)

const (
	// Annotation used to track the name of the snapshot being created
	snapshotAnnotation = "scribe.backube/snapname"
	// Time format for snapshot names and labels
	timeYYYYMMDDHHMMSS = "20060102150405"
)

type VolumeHandler struct {
	client                  client.Client
	owner                   metav1.Object
	copyMethod              scribev1alpha1.CopyMethodType
	capacity                *resource.Quantity
	storageClassName        *string
	accessModes             []v1.PersistentVolumeAccessMode
	volumeSnapshotClassName *string
}

// EnsurePVCFromSrc ensures the presence of a PVC that is based on the provided
// src PVC. It is generated based on the VolumeHandler's configuration. It may
// be the same PVC as src. Note: it's possible to return nil, nil. In this case,
// the operation should be retried.
func (vh *VolumeHandler) EnsurePVCFromSrc(ctx context.Context, log logr.Logger,
	src *v1.PersistentVolumeClaim, name string, isTemporary bool) (*v1.PersistentVolumeClaim, error) {
	switch vh.copyMethod {
	case scribev1alpha1.CopyMethodNone:
		return src, nil
	case scribev1alpha1.CopyMethodClone:
		return vh.ensureClone(ctx, log, src, name, isTemporary)
	case scribev1alpha1.CopyMethodSnapshot:
		snap, err := vh.ensureSnapshot(ctx, log, src, name, isTemporary)
		if snap == nil || err != nil {
			return nil, err
		}
		return vh.pvcFromSnapshot(ctx, log, snap, src, name, isTemporary)
	default:
		return nil, fmt.Errorf("unsupported copyMethod: %v -- must be None, Clone, or Snapshot", vh.copyMethod)
	}
}

// EnsureImage ensures the presence of a representation of the provided src
// PVC. It is generated based on the VolumeHandler's configuration and could be
// of type PersistentVolumeClaim or VolumeSnapshot. It may even be the same PVC
// as src.
func (vh *VolumeHandler) EnsureImage(ctx context.Context, log logr.Logger,
	src *v1.PersistentVolumeClaim) (*v1.TypedLocalObjectReference, error) {
	switch vh.copyMethod { //nolint: exhaustive
	case scribev1alpha1.CopyMethodNone:
		return &v1.TypedLocalObjectReference{
			APIGroup: &v1.SchemeGroupVersion.Group,
			Kind:     src.Kind,
			Name:     src.Name,
		}, nil
	case scribev1alpha1.CopyMethodSnapshot:
		snap, err := vh.ensureImageSnapshot(ctx, log, src)
		if snap == nil || err != nil {
			return nil, err
		}
		return &v1.TypedLocalObjectReference{
			APIGroup: &snapv1.SchemeGroupVersion.Group,
			Kind:     snap.Kind,
			Name:     snap.Name,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported copyMethod: %v -- must be None, or Snapshot", vh.copyMethod)
	}
}

func (vh *VolumeHandler) EnsureNewPVC(ctx context.Context, log logr.Logger,
	name string) (*v1.PersistentVolumeClaim, error) {
	// Ensure required configuration parameters have been provided in order to create volume
	if vh.accessModes == nil || len(vh.accessModes) == 0 {
		return nil, errors.New("accessModes must be provided when destinationPVC is not")
	}
	if vh.capacity == nil {
		return nil, errors.New("capacity must be provided when destinationPVC is not")
	}

	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: vh.owner.GetNamespace(),
		},
	}
	logger := log.WithValues("PVC", utils.NameFor(pvc))

	op, err := ctrlutil.CreateOrUpdate(ctx, vh.client, pvc, func() error {
		if err := ctrl.SetControllerReference(vh.owner, pvc, vh.client.Scheme()); err != nil {
			logger.Error(err, "unable to set controller reference")
			return err
		}
		if pvc.CreationTimestamp.IsZero() { // set immutable fields
			pvc.Spec.AccessModes = vh.accessModes
			pvc.Spec.StorageClassName = vh.storageClassName
			volumeMode := v1.PersistentVolumeFilesystem
			pvc.Spec.VolumeMode = &volumeMode
		}

		pvc.Spec.Resources.Requests = v1.ResourceList{
			v1.ResourceStorage: *vh.capacity,
		}
		return nil
	})

	if err != nil {
		logger.Error(err, "reconcile failed")
		return nil, err
	}
	logger.V(1).Info("PVC reconciled", "operation", op)
	return pvc, nil
}

func (vh *VolumeHandler) ensureImageSnapshot(ctx context.Context, log logr.Logger,
	src *v1.PersistentVolumeClaim) (*snapv1.VolumeSnapshot, error) {
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
		if err := ctrl.SetControllerReference(vh.owner, snap, vh.client.Scheme()); err != nil {
			logger.Error(err, "unable to set controller reference")
			return err
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

	// We only continue reconciling if the snapshot has been bound & not deleted
	if snap.Status == nil || snap.Status.BoundVolumeSnapshotContentName == nil || !snap.DeletionTimestamp.IsZero() {
		return nil, nil
	}

	return snap, nil
}

func (vh *VolumeHandler) ensureClone(ctx context.Context, log logr.Logger,
	src *v1.PersistentVolumeClaim, name string, isTemporary bool) (*v1.PersistentVolumeClaim, error) {
	clone := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: vh.owner.GetNamespace(),
		},
	}
	logger := log.WithValues("clone", utils.NameFor(clone))

	op, err := ctrlutil.CreateOrUpdate(ctx, vh.client, clone, func() error {
		if err := ctrl.SetControllerReference(vh.owner, clone, vh.client.Scheme()); err != nil {
			logger.Error(err, "unable to set controller reference")
			return err
		}
		if isTemporary {
			utils.MarkForCleanup(vh.owner, clone)
		}
		if clone.CreationTimestamp.IsZero() {
			if vh.capacity != nil {
				clone.Spec.Resources.Requests = v1.ResourceList{
					v1.ResourceStorage: *vh.capacity,
				}
			} else {
				clone.Spec.Resources.Requests = v1.ResourceList{
					v1.ResourceStorage: *src.Spec.Resources.Requests.Storage(),
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
			clone.Spec.DataSource = &v1.TypedLocalObjectReference{
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
	return clone, err
}

func (vh *VolumeHandler) ensureSnapshot(ctx context.Context, log logr.Logger,
	src *v1.PersistentVolumeClaim, name string, isTemporary bool) (*snapv1.VolumeSnapshot, error) {
	snap := &snapv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: vh.owner.GetNamespace(),
		},
	}
	logger := log.WithValues("snapshot", utils.NameFor(snap))

	op, err := ctrlutil.CreateOrUpdate(ctx, vh.client, snap, func() error {
		if err := ctrl.SetControllerReference(vh.owner, snap, vh.client.Scheme()); err != nil {
			logger.Error(err, "unable to set controller reference")
			return err
		}
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
	if snap.Status == nil || snap.Status.BoundVolumeSnapshotContentName == nil {
		logger.V(1).Info("waiting for snapshot to be bound")
		return nil, nil
	}
	logger.V(1).Info("temporary snapshot reconciled", "operation", op)
	return snap, nil
}

func (vh *VolumeHandler) pvcFromSnapshot(ctx context.Context, log logr.Logger,
	snap *snapv1.VolumeSnapshot, original *v1.PersistentVolumeClaim,
	name string, isTemporary bool) (*v1.PersistentVolumeClaim, error) {
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: vh.owner.GetNamespace(),
		},
	}
	logger := log.WithValues("pvc", utils.NameFor(pvc))

	op, err := ctrlutil.CreateOrUpdate(ctx, vh.client, pvc, func() error {
		if err := ctrl.SetControllerReference(vh.owner, pvc, vh.client.Scheme()); err != nil {
			logger.Error(err, "unable to set controller reference")
			return err
		}
		if isTemporary {
			utils.MarkForCleanup(vh.owner, pvc)
		}
		if pvc.CreationTimestamp.IsZero() {
			if vh.capacity != nil {
				pvc.Spec.Resources.Requests = v1.ResourceList{
					v1.ResourceStorage: *vh.capacity,
				}
			} else if snap.Status != nil && snap.Status.RestoreSize != nil {
				pvc.Spec.Resources.Requests = v1.ResourceList{
					v1.ResourceStorage: *snap.Status.RestoreSize,
				}
			} else {
				pvc.Spec.Resources.Requests = v1.ResourceList{
					v1.ResourceStorage: *original.Spec.Resources.Requests.Storage(),
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
			pvc.Spec.DataSource = &v1.TypedLocalObjectReference{
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
	logger.V(1).Info("pvc from snap reconciled", "operation", op)
	return pvc, nil
}
