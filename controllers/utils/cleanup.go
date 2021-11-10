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
	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1beta1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const cleanupLabelKey = "volsync.backube/cleanup"

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
	logger logr.Logger, owner metav1.Object, types []client.Object) error {
	uid := owner.GetUID()
	l := logger.WithValues("owned-by", uid)
	options := []client.DeleteAllOfOption{
		client.MatchingLabels{cleanupLabelKey: string(uid)},
		client.InNamespace(owner.GetNamespace()),
		client.PropagationPolicy(metav1.DeletePropagationBackground),
	}
	l.Info("deleting temporary objects")
	for _, obj := range types {
		err := c.DeleteAllOf(ctx, obj, options...)
		if client.IgnoreNotFound(err) != nil {
			l.Error(err, "unable to delete object(s)")
			return err
		}
	}
	return nil
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
