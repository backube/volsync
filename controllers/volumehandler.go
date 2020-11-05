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
	snapv1 "github.com/kubernetes-csi/external-snapshotter/v2/pkg/apis/volumesnapshot/v1beta1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	snapshotAnnotation = "scribe.backube/snapname"
)

type destinationVolumeHandler struct {
	ReplicationDestinationReconciler
	Ctx      context.Context
	Instance *scribev1alpha1.ReplicationDestination
	Options  *scribev1alpha1.ReplicationDestinationVolumeOptions
	PVC      *v1.PersistentVolumeClaim
	Snapshot *snapv1.VolumeSnapshot
}

func (h *destinationVolumeHandler) useProvidedPVC(l logr.Logger) (bool, error) {
	h.PVC = &v1.PersistentVolumeClaim{}
	pvcName := types.NamespacedName{Name: *h.Options.DestinationPVC, Namespace: h.Instance.Namespace}
	err := h.Client.Get(h.Ctx, pvcName, h.PVC)
	if err != nil {
		l.Error(err, "failed to get PVC with provided name", "PVC", pvcName)
		return false, err
	}
	return true, nil
}

// EnsurePVC ensures that there is a PVC available to replicate into. The PVC
// may be either user-provided or provisioned by this reconcile function. The
// resulting PVC to use will be available in h.PVC if this function returns
// successfully.
func (h *destinationVolumeHandler) EnsurePVC(l logr.Logger) (bool, error) {
	if h.Options.DestinationPVC != nil {
		return h.useProvidedPVC(l)
	}

	pvcName := types.NamespacedName{Name: "scribe-dest-" + h.Instance.Name, Namespace: h.Instance.Namespace}
	logger := l.WithValues("PVC", pvcName)
	// Ensure required configuration parameters have been provided in order to create volume
	if h.Options.AccessModes == nil || len(h.Options.AccessModes) == 0 {
		return false, errors.New("accessModes must be provided when destinationPVC is not")
	}
	if h.Options.Capacity == nil {
		return false, errors.New("capacity must be provided when destinationPVC is not")
	}

	h.PVC = &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName.Name,
			Namespace: pvcName.Namespace,
		},
	}
	// Note: we don't reconcile the immutable fields. We could do it by deleting
	// and recreating the PVC.
	op, err := ctrlutil.CreateOrUpdate(h.Ctx, h.Client, h.PVC, func() error {
		if err := ctrl.SetControllerReference(h.Instance, h.PVC, h.Scheme); err != nil {
			logger.Error(err, "unable to set controller reference")
			return err
		}
		if h.PVC.CreationTimestamp.IsZero() { // set immutable fields
			h.PVC.Spec.AccessModes = h.Options.AccessModes
			h.PVC.Spec.StorageClassName = h.Options.StorageClassName
			volumeMode := corev1.PersistentVolumeFilesystem
			h.PVC.Spec.VolumeMode = &volumeMode
		}

		h.PVC.Spec.Resources.Requests = corev1.ResourceList{
			corev1.ResourceStorage: *h.Options.Capacity,
		}
		return nil
	})

	if err != nil {
		logger.Error(err, "reconcile failed")
		return false, err
	}
	logger.V(1).Info("PVC reconciled", "operation", op)
	return true, nil
}

func (h *destinationVolumeHandler) createSnapshot(l logr.Logger) (bool, error) {
	// Track the name of the (in-progress) snapshot as a PVC annotation
	snapName := types.NamespacedName{Namespace: h.Instance.Namespace}
	if h.PVC.Annotations == nil {
		h.PVC.Annotations = make(map[string]string)
	}
	if name, ok := h.PVC.Annotations[snapshotAnnotation]; ok {
		snapName.Name = name
	} else {
		ts := time.Now().Format(timeYYYYMMDDHHMMSS)
		snapName.Name = "scribe-dest-" + h.Instance.Name + "-" + ts
		h.PVC.Annotations[snapshotAnnotation] = snapName.Name
		if err := h.Client.Update(h.Ctx, h.PVC); err != nil {
			l.Error(err, "unable to update PVC")
			return false, err
		}
		h.Instance.Status.LastSyncTime = &metav1.Time{Time: time.Now()}
	}
	logger := l.WithValues("snapshot", snapName)

	h.Snapshot = &snapv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      snapName.Name,
			Namespace: snapName.Namespace,
		},
	}
	op, err := ctrlutil.CreateOrUpdate(h.Ctx, h.Client, h.Snapshot, func() error {
		if err := ctrl.SetControllerReference(h.Instance, h.Snapshot, h.Scheme); err != nil {
			logger.Error(err, "unable to set controller reference")
			return err
		}
		if h.Snapshot.CreationTimestamp.IsZero() {
			h.Snapshot.Spec = snapv1.VolumeSnapshotSpec{
				Source: snapv1.VolumeSnapshotSource{
					PersistentVolumeClaimName: &h.PVC.Name,
				},
				VolumeSnapshotClassName: h.Options.VolumeSnapshotClassName,
			}
		}
		return nil
	})
	if err != nil {
		logger.Error(err, "reconcile failed")
		return false, err
	}
	logger.V(1).Info("Snapshot reconciled", "operation", op)

	// We only continue reconciling if the snapshot has been bound
	if h.Snapshot.Status == nil || h.Snapshot.Status.BoundVolumeSnapshotContentName == nil {
		return false, nil
	}
	return true, nil
}

func (h *destinationVolumeHandler) cleanupOldSnapshot(l logr.Logger) (bool, error) {
	// Make sure we only delete an old snapshot (it's a snapshot, but not the
	// current one)
	if h.Instance.Status.LatestImage == nil ||
		h.Instance.Status.LatestImage.Kind != "VolumeSnapshot" ||
		*h.Instance.Status.LatestImage.APIGroup != snapv1.SchemeGroupVersion.Group ||
		h.Instance.Status.LatestImage.Name == h.Snapshot.Name {
		l.V(1).Info("No Snap to clean up.", "latest", h.Instance.Status.LatestImage)
		return true, nil
	}

	oldSnap := &snapv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      h.Instance.Status.LatestImage.Name,
			Namespace: h.Instance.Namespace,
		},
	}
	l.V(1).Info("deleting old snapshot", "snapshot", oldSnap)
	err := h.Client.Delete(h.Ctx, oldSnap)
	if err != nil && !kerrors.IsNotFound(err) {
		l.Error(err, "unable to delete old snapshot")
		return false, err
	}
	// Don't need to force the status update
	h.Instance.Status.LatestImage = nil
	return true, nil
}

func (h *destinationVolumeHandler) recordNewSnapshot(l logr.Logger) (bool, error) {
	h.Instance.Status.LatestImage = &v1.TypedLocalObjectReference{
		APIGroup: &snapv1.SchemeGroupVersion.Group,
		Kind:     h.Snapshot.Kind,
		Name:     h.Snapshot.Name,
	}
	err := h.Status().Update(h.Ctx, h.Instance)
	if err != nil {
		l.Error(err, "unable to save snapshot name")
		return false, err
	}
	return true, nil
}

func (h *destinationVolumeHandler) removeSnapshotAnnotation(l logr.Logger) (bool, error) {
	delete(h.PVC.Annotations, snapshotAnnotation)
	if err := h.Client.Update(h.Ctx, h.PVC); err != nil {
		l.Error(err, "unable to remove snapshot annotation from PVC")
		return false, err
	}
	return true, nil
}

// PreserveImage implements the methods for preserving a PiT copy of the
// replicated data.
func (h *destinationVolumeHandler) PreserveImage(l logr.Logger) (bool, error) {
	// If using "None", there's nothing to do
	if h.Options.CopyMethod == scribev1alpha1.CopyMethodNone {
		return true, nil
	}
	if h.Options.CopyMethod != scribev1alpha1.CopyMethodSnapshot {
		return false, fmt.Errorf("unsupported copyMethod: %v -- must be None or Snapshot", h.Options.CopyMethod)
	}

	// Preserve the data via snapshot
	return reconcileBatch(l,
		h.createSnapshot,
		h.cleanupOldSnapshot,
		h.recordNewSnapshot,
		h.removeSnapshotAnnotation,
	)
}
