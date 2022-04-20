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

package utils

import (
	"context"

	"github.com/go-logr/logr"
	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	volsyncLabelPrefix    = "volsync.backube"
	cleanupLabelKey       = volsyncLabelPrefix + "/cleanup"
	DoNotDeleteLabelKey   = volsyncLabelPrefix + "/do-not-delete"
	DoNotDeleteLabelValue = "true"
)

// MarkForCleanup marks the provided "obj" to be deleted at the end of the
// synchronization iteration.
func MarkForCleanup(owner metav1.Object, obj metav1.Object) {
	uid := owner.GetUID()
	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[cleanupLabelKey] = string(uid)
	obj.SetLabels(labels)
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
		snapForCleanup := &snapList.Items[i]

		// Use a delete precondition to avoid timing issues.
		// If the object was modified (for example by someone adding a new label) in-between us loading it and
		// performing the delete, the should throw an error as the resourceVersion will not match
		snapResourceVersion := snapForCleanup.ResourceVersion

		deleteOptions := &client.DeleteOptions{
			Preconditions: &metav1.Preconditions{
				ResourceVersion: &snapResourceVersion,
			},
		}

		err := c.Delete(ctx, snapForCleanup, deleteOptions)
		if err != nil {
			return err
		}
	}

	return nil
}

func RelinquishOwnedSnapshotsWithDoNotDeleteLabel(ctx context.Context, c client.Client,
	logger logr.Logger, owner client.Object) error {
	// Find all snapshots in the namespace with the do not delete label
	listOptions := []client.ListOption{
		client.MatchingLabels{DoNotDeleteLabelKey: DoNotDeleteLabelValue},
		client.InNamespace(owner.GetNamespace()),
	}
	snapList := &snapv1.VolumeSnapshotList{}
	err := c.List(ctx, snapList, listOptions...)
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
	for i := range snapList.Items {
		snapshot := snapList.Items[i]

		ownershipRemoved, err := RemoveSnapshotOwnershipIfRequestedAndUpdate(ctx, c, logger,
			owner, &snapshot)
		if err != nil {
			return remainingSnapshots, err
		}
		if !ownershipRemoved {
			remainingSnapshots = append(remainingSnapshots, snapshot)
		}
	}

	return remainingSnapshots, nil
}

func RemoveSnapshotOwnershipIfRequestedAndUpdate(ctx context.Context, c client.Client, logger logr.Logger,
	owner client.Object, snapshot *snapv1.VolumeSnapshot) (bool, error) {
	ownershipRemoved := false

	if val, ok := snapshot.Labels[DoNotDeleteLabelKey]; ok && val == DoNotDeleteLabelValue {
		logger.Info("Not deleting volumesnapshot protected with label - will remove ownership and cleanup label",
			"name", snapshot.GetName(), "label", DoNotDeleteLabelKey)
		ownershipRemoved = true

		updated := unMarkForCleanupAndRemoveOwnership(c.Scheme(), snapshot, owner)
		if updated {
			err := c.Update(ctx, snapshot)
			if err != nil {
				logger.Error(err, "error removing cleanup label or ownerRef from snapshot",
					"name", snapshot.GetName(), "namespace", snapshot.GetNamespace())
				return ownershipRemoved, err
			}
		}
	}

	return ownershipRemoved, nil
}

func unMarkForCleanupAndRemoveOwnership(scheme *runtime.Scheme, obj metav1.Object, owner client.Object) bool {
	updated := false

	// Remove volsync cleanup label if present
	updatedLabels := obj.GetLabels()
	if _, ok := updatedLabels[cleanupLabelKey]; ok {
		delete(updatedLabels, cleanupLabelKey)
		updated = true
	}

	ownerKindAndName := KindAndName(scheme, owner)

	// Remove ReplicationDestination owner reference if present
	updatedOwnerRefs := []metav1.OwnerReference{}
	for _, ownerRef := range obj.GetOwnerReferences() {
		if ownerRef.Kind+"/"+ownerRef.Name == ownerKindAndName {
			// Do not add to updatedOwnerRefs
			updated = true
		} else {
			updatedOwnerRefs = append(updatedOwnerRefs, ownerRef)
		}
	}

	if updated {
		obj.SetLabels(updatedLabels)
		obj.SetOwnerReferences(updatedOwnerRefs)

		return true
	}
	return false
}

func MarkOldSnapshotForCleanup(ctx context.Context, c client.Client, logger logr.Logger,
	owner metav1.Object, oldImage, latestImage *corev1.TypedLocalObjectReference) error {
	// Make sure we only delete an old snapshot (it's a snapshot, but not the
	// current one)

	// There's no latestImage or type != snapshot
	if !isSnapshot(latestImage) {
		return nil
	}
	// No oldImage or type != snapshot
	if !isSnapshot(oldImage) {
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

func isSnapshot(image *corev1.TypedLocalObjectReference) bool {
	if image == nil {
		return false
	}
	if image.Kind != "VolumeSnapshot" || *image.APIGroup != snapv1.SchemeGroupVersion.Group {
		return false
	}
	return true
}
