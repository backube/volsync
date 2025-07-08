/*
Copyright 2024 The VolSync authors.

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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

func PVCUsesCopyTrigger(pvc *corev1.PersistentVolumeClaim) bool {
	// If the annotation exists on the PVC (with any value) then we assume copy-trigger coordination is desired
	_, ok := pvc.Annotations[volsyncv1alpha1.UseCopyTriggerAnnotation]
	return ok
}

func GetCopyTriggerValue(pvc *corev1.PersistentVolumeClaim) string {
	return pvc.Annotations[volsyncv1alpha1.CopyTriggerAnnotation]
}

func GetLatestCopyTriggerValue(pvc *corev1.PersistentVolumeClaim) string {
	return pvc.Annotations[volsyncv1alpha1.LatestCopyTriggerAnnotation]
}

func GetLatestCopyTriggerWaitingSinceValue(pvc *corev1.PersistentVolumeClaim) string {
	return pvc.Annotations[volsyncv1alpha1.LatestCopyTriggerWaitingSinceAnnotation]
}

// Returns true if the pvc was updated
func SetLatestCopyTriggerWaitingSinceValueNow(pvc *corev1.PersistentVolumeClaim) bool {
	return setAnnotation(pvc,
		volsyncv1alpha1.LatestCopyTriggerWaitingSinceAnnotation, time.Now().UTC().Format(time.RFC3339))
}

// Returns true if the pvc was updated
func SetLatestCopyStatusWaitingForTrigger(pvc *corev1.PersistentVolumeClaim) bool {
	updated := setAnnotation(pvc,
		volsyncv1alpha1.LatestCopyStatusAnnotation, volsyncv1alpha1.LatestCopyStatusValueWaitingForTrigger)
	if updated || GetLatestCopyTriggerWaitingSinceValue(pvc) == "" {
		// We're setting the status to WaitingForTrigger - set a waiting-since annotation
		// to give us a timestamp
		updated = SetLatestCopyTriggerWaitingSinceValueNow(pvc)
	}

	return updated
}

// Returns true if the pvc was updated
func SetLatestCopyStatusInProgress(pvc *corev1.PersistentVolumeClaim) bool {
	updated := setAnnotation(pvc,
		volsyncv1alpha1.LatestCopyStatusAnnotation, volsyncv1alpha1.LatestCopyStatusValueInProgress)
	// Make sure waiting since annotation is removed
	return unsetAnnotation(pvc, volsyncv1alpha1.LatestCopyTriggerWaitingSinceAnnotation) || updated
}

// Returns true if the pvc was updated
func SetLatestCopyStatusCompleted(pvc *corev1.PersistentVolumeClaim) bool {
	updated := setAnnotation(pvc,
		volsyncv1alpha1.LatestCopyStatusAnnotation, volsyncv1alpha1.LatestCopyStatusValueCompleted)
	// Make sure waiting since annotation is removed
	return unsetAnnotation(pvc, volsyncv1alpha1.LatestCopyTriggerWaitingSinceAnnotation) || updated
}

// Returns true if the pvc was updated
func SetLatestCopyTriggerValue(pvc *corev1.PersistentVolumeClaim, value string) bool {
	return setAnnotation(pvc, volsyncv1alpha1.LatestCopyTriggerAnnotation, value)
}

// Returns true if the local obj resource was updated - does not perform a client.Update()
func setAnnotation(obj metav1.Object, annotationName string, annotationValue string) bool {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}

	existingAnnotationValue, ok := annotations[annotationName]
	if ok && existingAnnotationValue == annotationValue {
		return false // Nothing to update
	}

	// Set the annotation and update
	annotations[annotationName] = annotationValue
	obj.SetAnnotations(annotations)

	return true // updated
}

func unsetAnnotation(obj metav1.Object, annotationName string) bool {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}

	_, ok := annotations[annotationName]
	if !ok {
		// Nothing to remove
		return false
	}

	delete(annotations, annotationName)
	obj.SetAnnotations(annotations)

	return true // updated
}
