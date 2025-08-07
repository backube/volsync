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

//nolint:revive
package utils

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/internal/controller/mover"
)

// MarkForCleanup marks the provided "obj" to be deleted at the end of the
// synchronization iteration.
func MarkForCleanup(owner metav1.Object, obj metav1.Object) bool {
	uid := owner.GetUID()
	return AddLabel(obj, cleanupLabelKey, string(uid))
}

// UnmarkForCleanup removes any previously applied cleanup label
func UnmarkForCleanup(obj metav1.Object) bool {
	return RemoveLabel(obj, cleanupLabelKey)
}

// CleanupObjects deletes all objects that have been marked. The objects to be
// cleaned up must have been previously marked via MarkForCleanup() and
// associated with "owner". The "types" array should contain one object of each
// type to clean up.
func CleanupObjects(ctx context.Context, c client.Client,
	logger logr.Logger, owner client.Object, types []client.Object) error {
	uid := owner.GetUID()
	l := logger.WithValues("owned-by", uid)
	options := []client.DeleteAllOfOption{
		client.MatchingLabels{cleanupLabelKey: string(uid)},
		client.InNamespace(owner.GetNamespace()),
		client.PropagationPolicy(metav1.DeletePropagationBackground),
	}
	l.Info("deleting temporary objects")
	for _, obj := range types {
		_, ok := obj.(*snapv1.VolumeSnapshot)
		if ok {
			// Handle volumesnapshots differently - do not delete if they have a specific label
			err := cleanupSnapshots(ctx, c, logger, owner)
			if err != nil {
				l.Error(err, "unable to delete volume snapshot(s)")
				return err
			}
		} else {
			err := c.DeleteAllOf(ctx, obj, options...)
			if client.IgnoreNotFound(err) != nil {
				l.Error(err, "unable to delete object(s)")
				return err
			}
		}
	}
	return nil
}

// Could be generalized to other types if we want to use unstructuredList - would need to pass in group, version, kind
func cleanupSnapshots(ctx context.Context, c client.Client,
	logger logr.Logger, owner client.Object) error {
	// Load current list of snapshots with the cleanup label
	listOptions := []client.ListOption{
		client.MatchingLabels{cleanupLabelKey: string(owner.GetUID())},
		client.InNamespace(owner.GetNamespace()),
	}
	snapList := &snapv1.VolumeSnapshotList{}
	err := c.List(ctx, snapList, listOptions...)
	if err != nil {
		return err
	}

	return CleanupSnapshotsWithLabelCheck(ctx, c, logger, owner, snapList)
}

func CleanupSnapshotsWithLabelCheck(ctx context.Context, c client.Client,
	logger logr.Logger, owner client.Object, snapList *snapv1.VolumeSnapshotList) error {
	// If marked as do-not-delete, remove the cleanup label and ownership
	snapsForCleanup, err := relinquishSnapshotsWithDoNotDeleteLabel(ctx, c, logger, owner, snapList)
	if err != nil {
		return err
	}

	// Remaining snapshots should be cleaned up
	for i := range snapsForCleanup {
		snapForCleanup := &snapsForCleanup[i]

		if snapInUseByOther(snapForCleanup, owner) {
			// If the snapshot has any other owner reference or used by vol populator pvc
			// while provisioning, then do not delete just remove our own owner reference
			updated := RemoveOwnerReference(snapForCleanup, owner)
			if updated {
				err := c.Update(ctx, snapForCleanup)
				if err != nil {
					logger.Error(err, "error removing ownerRef from snapshot",
						"name", snapForCleanup.GetName(), "namespace", snapForCleanup.GetNamespace())
					return err
				}
			}
		} else {
			// Use a delete precondition to avoid timing issues.
			// If the object was modified (for example by someone adding a new label) in-between us loading it and
			// performing the delete, the should throw an error as the resourceVersion will not match
			err := c.Delete(ctx, snapForCleanup, client.Preconditions{ResourceVersion: &snapForCleanup.ResourceVersion})
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func RelinquishOwnedSnapshotsWithDoNotDeleteLabel(ctx context.Context, c client.Client,
	logger logr.Logger, owner client.Object) error {
	// Find all snapshots in the namespace with the do not delete label
	ls, err := labels.Parse(DoNotDeleteLabelKey)
	if err != nil {
		return err
	}

	listOptions := []client.ListOption{
		client.MatchingLabelsSelector{
			Selector: ls,
		},
		client.InNamespace(owner.GetNamespace()),
	}
	snapList := &snapv1.VolumeSnapshotList{}
	err = c.List(ctx, snapList, listOptions...)
	if err != nil {
		return err
	}

	_, err = relinquishSnapshotsWithDoNotDeleteLabel(ctx, c, logger, owner, snapList)
	return err
}

// Returns a list of remaining VolumeSnapshots that were not relinquished
func relinquishSnapshotsWithDoNotDeleteLabel(ctx context.Context, c client.Client,
	logger logr.Logger, owner client.Object,
	snapList *snapv1.VolumeSnapshotList) ([]snapv1.VolumeSnapshot, error) {
	remainingSnapshots := []snapv1.VolumeSnapshot{}

	var snapRelinquishErr error

	for i := range snapList.Items {
		snapshot := snapList.Items[i]

		ownershipRemoved, err := RemoveSnapOwnershipAndLabelsIfRequestedAndUpdate(ctx, c, logger,
			owner, &snapshot)
		if err != nil {
			snapRelinquishErr = err // Will return the latest error at the end but keep processing the snaps
			continue
		}
		if !ownershipRemoved {
			remainingSnapshots = append(remainingSnapshots, snapshot)
		}
	}

	return remainingSnapshots, snapRelinquishErr
}

func RemoveSnapOwnershipAndLabelsIfRequestedAndUpdate(ctx context.Context, c client.Client, logger logr.Logger,
	owner client.Object, snapshot *snapv1.VolumeSnapshot) (bool, error) {
	ownershipRemoved := false

	if IsMarkedDoNotDelete(snapshot) {
		logger.Info("Not deleting volumesnapshot protected with label - will remove ownership and cleanup label",
			"name", snapshot.GetName(), "label", DoNotDeleteLabelKey)
		ownershipRemoved = true

		updated := UnMarkForCleanupAndRemoveOwnership(snapshot, owner)
		if updated {
			err := c.Update(ctx, snapshot)
			if err != nil {
				logger.Error(err, "error removing cleanup label or ownerRef from snapshot",
					"name", snapshot.GetName(), "namespace", snapshot.GetNamespace())
				return false, err
			}
		}
	}

	return ownershipRemoved, nil
}

func IsMarkedDoNotDelete(snapshot *snapv1.VolumeSnapshot) bool {
	return HasLabel(snapshot, DoNotDeleteLabelKey)
}

func MarkDoNotDelete(snapshot *snapv1.VolumeSnapshot) bool {
	return AddLabel(snapshot, DoNotDeleteLabelKey, "true")
}

func UnMarkForCleanupAndRemoveOwnership(obj metav1.Object, owner client.Object) bool {
	updated := false

	// Remove volsync cleanup label & ownership label if present
	updated = UnmarkForCleanup(obj) || updated
	updated = RemoveOwnedByVolSync(obj) || updated

	// Remove ReplicationDestination owner reference if present
	return RemoveOwnerReference(obj, owner) || updated
}

func RemoveOwnerReference(obj metav1.Object, owner client.Object) bool {
	updated := false
	updatedOwnerRefs := []metav1.OwnerReference{}
	for _, ownerRef := range obj.GetOwnerReferences() {
		if ownerRef.UID == owner.GetUID() {
			// Do not add to updatedOwnerRefs
			updated = true
		} else {
			updatedOwnerRefs = append(updatedOwnerRefs, ownerRef)
		}
	}
	if updated {
		obj.SetOwnerReferences(updatedOwnerRefs)
	}

	return updated
}

func snapInUseByOther(snapshot *snapv1.VolumeSnapshot, owner client.Object) bool {
	return hasOtherOwnerRef(snapshot, owner) || snapInUseByVolumePopulatorPVC(snapshot)
}

func snapInUseByVolumePopulatorPVC(snapshot *snapv1.VolumeSnapshot) bool {
	// Volume Populator will put on a label with a specific prefix on a snapshot while
	// it's populating the PVC from that snapshot - this indicates at least one pvc for
	// the volume populator is actively using this snapshot
	for _, labelKey := range snapshot.GetLabels() {
		if strings.HasPrefix(labelKey, SnapInUseByVolumePopulatorLabelPrefix) {
			return true
		}
	}
	return false
}

func hasOtherOwnerRef(obj metav1.Object, owner client.Object) bool {
	for _, ownerRef := range obj.GetOwnerReferences() {
		if ownerRef.UID != owner.GetUID() {
			return true
		}
	}
	return false
}

func MarkOldSnapshotForCleanup(ctx context.Context, c client.Client, logger logr.Logger,
	owner metav1.Object, oldImage, latestImage *corev1.TypedLocalObjectReference) error {
	// Make sure we only delete an old snapshot (it's a snapshot, but not the
	// current one)

	// There's no latestImage or type != snapshot
	if !IsSnapshot(latestImage) {
		return nil
	}
	// No oldImage or type != snapshot
	if !IsSnapshot(oldImage) {
		return nil
	}

	// Also don't clean it up if it's the snap we're trying to preserve
	if latestImage.Name == oldImage.Name {
		return nil
	}

	oldSnap := &snapv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      oldImage.Name,
			Namespace: owner.GetNamespace(),
		},
	}
	err := c.Get(ctx, client.ObjectKeyFromObject(oldSnap), oldSnap)
	if kerrors.IsNotFound(err) {
		// Nothing to cleanup
		return nil
	}
	if err != nil {
		logger.Error(err, "unable to get snapshot", "name", oldSnap.GetName(), "namespace", oldSnap.GetNamespace())
		return err
	}

	if IsMarkedDoNotDelete(oldSnap) {
		logger.Info("Snapshot is marked do-not-delete, will not mark for cleanup")
		return nil
	}

	// Update the old snapshot with the cleanup label
	MarkForCleanup(owner, oldSnap)
	err = c.Update(ctx, oldSnap)
	if err != nil && !kerrors.IsNotFound(err) {
		logger.Error(err, "unable to update snapshot with cleanup label",
			"name", oldSnap.GetName(), "namespace", oldSnap.GetNamespace())
		return err
	}
	return nil
}

func IsSnapshot(image *corev1.TypedLocalObjectReference) bool {
	if image == nil {
		return false
	}
	if image.Kind != "VolumeSnapshot" || image.APIGroup == nil || *image.APIGroup != snapv1.SchemeGroupVersion.Group {
		return false
	}
	return true
}

// MoverCleanupObject represents an object that can be cleaned up by movers
type MoverCleanupObject interface {
	client.Object
}

// MoverFactory creates a mover for cleanup operations
type MoverFactory func(
	obj MoverCleanupObject,
	c client.Client,
	logger logr.Logger,
	er events.EventRecorder,
	privilegedMoverOk bool,
) (mover.Mover, error)

// CleanupMoverResources handles cleanup of mover-specific resources for any replication object
func CleanupMoverResources(ctx context.Context, obj MoverCleanupObject, c client.Client, logger logr.Logger,
	eventRecorder events.EventRecorder, moverFactory MoverFactory) (ctrl.Result, error) {
	// Check if privileged movers are allowed via namespace annotation
	privilegedMoverOk, err := PrivilegedMoversOk(ctx, c, logger, obj.GetNamespace())
	if err != nil {
		logger.Error(err, "Failed to check privileged mover permissions during cleanup")
		// Continue with cleanup even if this check fails
	}

	// Try to create mover for cleanup - this might fail if mover is not found, which is ok during deletion
	dataMover, err := moverFactory(obj, c, logger, eventRecorder, privilegedMoverOk)

	if err != nil && !errors.Is(err, mover.ErrNoMoverFound) {
		logger.Error(err, "Failed to create mover during deletion, but continuing cleanup")
	}

	// If we have a mover, try to clean up mover-specific resources
	if dataMover != nil {
		result, cleanupErr := dataMover.Cleanup(ctx)
		if cleanupErr != nil {
			logger.Error(cleanupErr, "Error during mover cleanup")
			return ctrl.Result{RequeueAfter: time.Second * 30}, cleanupErr
		}

		// If cleanup is not complete, requeue
		if !result.Completed {
			logger.Info("Mover cleanup in progress, requeuing")
			return ctrl.Result{RequeueAfter: time.Second * 10}, nil
		}
	}

	return ctrl.Result{}, nil
}

// CreateReplicationDestinationMoverFactory creates a pre-built mover factory for ReplicationDestination cleanup
func CreateReplicationDestinationMoverFactory() MoverFactory {
	return func(obj MoverCleanupObject, c client.Client, logger logr.Logger,
		er events.EventRecorder, privilegedMoverOk bool) (mover.Mover, error) {
		if obj == nil {
			return nil, errors.New("object is nil")
		}
		rd, ok := obj.(*volsyncv1alpha1.ReplicationDestination)
		if !ok {
			return nil, fmt.Errorf("object is not a ReplicationDestination, got %T", obj)
		}
		// Create destination machine using the destination mover catalog
		dataMover, err := mover.GetDestinationMoverFromCatalog(c, logger, er, rd, privilegedMoverOk)
		if err != nil {
			return nil, fmt.Errorf("failed to get destination mover from catalog: %w", err)
		}
		return dataMover, nil
	}
}

// CreateReplicationSourceMoverFactory creates a pre-built mover factory for ReplicationSource cleanup
func CreateReplicationSourceMoverFactory() MoverFactory {
	return func(obj MoverCleanupObject, c client.Client, logger logr.Logger,
		er events.EventRecorder, privilegedMoverOk bool) (mover.Mover, error) {
		if obj == nil {
			return nil, errors.New("object is nil")
		}
		rs, ok := obj.(*volsyncv1alpha1.ReplicationSource)
		if !ok {
			return nil, fmt.Errorf("object is not a ReplicationSource, got %T", obj)
		}
		// Create source machine using the source mover catalog
		dataMover, err := mover.GetSourceMoverFromCatalog(c, logger, er, rs, privilegedMoverOk)
		if err != nil {
			return nil, fmt.Errorf("failed to get source mover from catalog: %w", err)
		}
		return dataMover, nil
	}
}
