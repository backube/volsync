/*
Copyright 2022 The VolSync authors.

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
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AffinityInfo struct {
	NodeName    string
	Tolerations []corev1.Toleration
}

// Determine the proper affinity to apply based on the current users of a PVC
func AffinityFromVolume(ctx context.Context, c client.Client, logger logr.Logger,
	pvc *corev1.PersistentVolumeClaim) (*AffinityInfo, error) {
	if pvc == nil {
		err := fmt.Errorf("can't determine affinity for a nil PVC")
		logger.Error(err, "unable to determine affinity")
		return nil, err
	}

	// If it's an RWX, it doesn't matter where we schedule
	for _, am := range pvc.Status.AccessModes {
		if am == corev1.ReadWriteMany {
			return &AffinityInfo{}, nil
		}
	}

	// Find all the Pods that are using the PVC
	podsUsing, err := podsUsingPVC(ctx, c, logger, pvc)
	if err != nil {
		return nil, err
	}

	// Loop through all the volumes and find:
	// - A running Pod using the volume
	// - A pending Pod using the volume (if none are running)
	// Do this for both non-VolSync owned Pods as well as VolSync-owned
	var candidatePod *corev1.Pod
	var volsyncUsingPod *corev1.Pod
	for i := range podsUsing {
		pod := &podsUsing[i] // Not allocated in range stmt to avoid pointer aliasing
		if IsOwnedByVolsync(pod) {
			if (pod.Status.Phase == corev1.PodRunning) ||
				(pod.Status.Phase == corev1.PodPending && volsyncUsingPod == nil) {
				volsyncUsingPod = pod
			}
		} else {
			if (pod.Status.Phase == corev1.PodRunning) ||
				(pod.Status.Phase == corev1.PodPending && candidatePod == nil) {
				candidatePod = pod
			}
		}
	}

	// If there aren't any "real" users of the PVC, fall back to determining
	// affinity using VolSync owned Pods, too. We have this fallback in case
	// there are multiple RS that reference this PVC... Those would need to be
	// co-scheduled.
	if candidatePod == nil {
		candidatePod = volsyncUsingPod
	}

	if candidatePod == nil {
		// Nobody is using the volume
		return &AffinityInfo{}, nil
	}

	affinity := AffinityInfo{
		NodeName:    candidatePod.Spec.NodeName,
		Tolerations: candidatePod.Spec.Tolerations,
	}

	return &affinity, nil
}

// Find all the Pods using a PVC
func podsUsingPVC(ctx context.Context, c client.Client, logger logr.Logger,
	pvc *corev1.PersistentVolumeClaim) ([]corev1.Pod, error) {
	podList := corev1.PodList{}
	if err := c.List(ctx, &podList, client.InNamespace(pvc.Namespace)); err != nil {
		return nil, err
	}

	podsUsing := []corev1.Pod{}
	for _, pod := range podList.Items {
		for _, volume := range pod.Spec.Volumes {
			if volume.PersistentVolumeClaim != nil &&
				volume.PersistentVolumeClaim.ClaimName == pvc.Name {
				podsUsing = append(podsUsing, pod)
			}
		}
	}

	return podsUsing, nil
}
