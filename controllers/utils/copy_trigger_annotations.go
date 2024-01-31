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

package utils

import (
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// Annotation optionally set on src pvc by user.  When set, a volsync source replication
	// that is using CopyMode: Snapshot or Clone will wait for the user to set a unique copy-trigger
	// before proceeding to take the src snapshot/clone.
	UseCopyTriggerAnnotation = "volsync.backube/use-copy-trigger"
	SnapTriggerAnnotation    = "volsync.backube/copy-trigger"

	// Annotations for status set by VolSync on a src pvc if UseCopyTriggerAnnotation is set to "true"
	LatestCopyTriggerAnnotation             = "volsync.backube/latest-copy-trigger"
	LatestCopyStatusAnnotation              = "volsync.backube/latest-copy-status"
	LatestCopyTriggerWaitingSinceAnnotation = "volsync.backube/latest-copy-trigger-waiting-since"

	// VolSync latest-copy-status annotation values
	LatestCopyStatusValueWaitingForTrigger = "WaitingForTrigger"
	LatestCopyStatusValueCreating          = "InProgress"
	LatestCopyStatusValueCompleted         = "Completed"

	CopyTriggerWaitTimeout time.Duration = 10 * time.Minute
)

func PVCUsesCopyTrigger(pvc *corev1.PersistentVolumeClaim) bool {
	copyTriggerAnnotationValue, ok := pvc.Annotations[UseCopyTriggerAnnotation]
	if !ok {
		return false
	}

	// If the annotation exists and is not false/FALSE/no/NO then we assume
	// it uses the copy trigger
	if strings.EqualFold(copyTriggerAnnotationValue, "false") ||
		strings.EqualFold(copyTriggerAnnotationValue, "no") {
		return false
	}

	return true
}

func GetCopyTriggerValue(pvc *corev1.PersistentVolumeClaim) string {
	return pvc.Annotations[SnapTriggerAnnotation]
}

func GetLatestCopyTriggerValue(pvc *corev1.PersistentVolumeClaim) string {
	return pvc.Annotations[LatestCopyTriggerAnnotation]
}

func GetLatestCopyTriggerWaitingSinceValue(pvc *corev1.PersistentVolumeClaim) string {
	return pvc.Annotations[LatestCopyTriggerWaitingSinceAnnotation]
}

// Returns true if the pvc was updated
func SetLatestCopyTriggerWaitingSinceValueNow(pvc *corev1.PersistentVolumeClaim) bool {
	return setAnnotation(pvc, LatestCopyTriggerWaitingSinceAnnotation, time.Now().UTC().Format(time.RFC3339))
}

// Returns true if the pvc was updated
func SetLatestCopyStatusWaitingForTrigger(pvc *corev1.PersistentVolumeClaim) bool {
	updated := setAnnotation(pvc, LatestCopyStatusAnnotation, LatestCopyStatusValueWaitingForTrigger)
	if updated || GetLatestCopyTriggerWaitingSinceValue(pvc) == "" {
		// We're setting the status to WaitingForTrigger - set a waiting-since annotation
		// to give us a timestamp
		updated = SetLatestCopyTriggerWaitingSinceValueNow(pvc)
	}

	return updated
}

// Returns true if the pvc was updated
func SetLatestCopyStatusInProgress(pvc *corev1.PersistentVolumeClaim) bool {
	updated := setAnnotation(pvc, LatestCopyStatusAnnotation, LatestCopyStatusValueCreating)
	// Make sure waiting since annotation is removed
	return unsetAnnotation(pvc, LatestCopyTriggerWaitingSinceAnnotation) || updated
}

// Returns true if the pvc was updated
func SetLatestCopyStatusCompleted(pvc *corev1.PersistentVolumeClaim) bool {
	updated := setAnnotation(pvc, LatestCopyStatusAnnotation, LatestCopyStatusValueCompleted)
	// Make sure waiting since annotation is removed
	return unsetAnnotation(pvc, LatestCopyTriggerWaitingSinceAnnotation) || updated
}

// Returns true if the pvc was updated
func SetLatestCopyTriggerValue(pvc *corev1.PersistentVolumeClaim, value string) bool {
	return setAnnotation(pvc, LatestCopyTriggerAnnotation, value)
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
	return true
}
