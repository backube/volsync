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
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// reconcileFunc is a function that partially reconciles an object. It returns a
// bool indicating whether reconciling should continue and an error.
type ReconcileFunc func(logr.Logger) (bool, error)

// reconcileBatch steps through a list of reconcile functions until one returns
// false or an error.
func ReconcileBatch(l logr.Logger, reconcileFuncs ...ReconcileFunc) (bool, error) {
	for _, f := range reconcileFuncs {
		if cont, err := f(l); !cont || err != nil {
			return cont, err
		}
	}
	return true, nil
}

// JobHasActiveOrTerminatingPods reports whether a Job still has Pods that must
// finish before the Job can be treated as complete or replaced. If neither
// status count is positive, it lists Pods with the uncached API reader,
// including when Terminating is explicitly zero. This catches stale status
// during deletion and terminating Pods omitted by older Kubernetes versions.
// It returns true with an error when it cannot verify the Pod state.
func JobHasActiveOrTerminatingPods(ctx context.Context, apiReader client.Reader, job *batchv1.Job) (bool, error) {
	if job == nil {
		return false, nil
	}

	if job.Status.Active > 0 || (job.Status.Terminating != nil && *job.Status.Terminating > 0) {
		return true, nil
	}

	pods, err := listJobPods(ctx, apiReader, job)
	if err != nil {
		return true, err
	}

	return hasNonterminalOwnedPods(pods, job), nil
}

func listJobPods(ctx context.Context, apiReader client.Reader, job *batchv1.Job) (*corev1.PodList, error) {
	if apiReader == nil {
		return nil, fmt.Errorf("unable to verify Pods for Job %s/%s: API reader is nil", job.Namespace, job.Name)
	}
	if job.Namespace == "" {
		return nil, fmt.Errorf("unable to verify Pods for Job %s without a namespace", job.Name)
	}
	if job.UID == "" {
		return nil, fmt.Errorf("unable to verify Pods for Job %s/%s without a UID", job.Namespace, job.Name)
	}
	if job.Spec.Selector == nil ||
		(len(job.Spec.Selector.MatchLabels) == 0 && len(job.Spec.Selector.MatchExpressions) == 0) {
		return nil, fmt.Errorf("unable to verify Pods for Job %s/%s without a selector", job.Namespace, job.Name)
	}

	selector, err := metav1.LabelSelectorAsSelector(job.Spec.Selector)
	if err != nil {
		return nil, fmt.Errorf("unable to parse selector for Job %s/%s: %w", job.Namespace, job.Name, err)
	}

	pods := &corev1.PodList{}
	listOptions := []client.ListOption{
		client.InNamespace(job.Namespace),
		client.MatchingLabelsSelector{Selector: selector},
	}
	if err := apiReader.List(ctx, pods, listOptions...); err != nil {
		return nil, fmt.Errorf("unable to list Pods for Job %s/%s: %w", job.Namespace, job.Name, err)
	}

	return pods, nil
}

func hasNonterminalOwnedPods(pods *corev1.PodList, job *batchv1.Job) bool {
	for i := range pods.Items {
		pod := &pods.Items[i]
		if !metav1.IsControlledBy(pod, job) {
			continue
		}
		if pod.Status.Phase != corev1.PodSucceeded && pod.Status.Phase != corev1.PodFailed {
			return true
		}
	}

	return false
}

// JobShouldWaitForCompletion reports whether a mover Job is still in progress.
// Deferred updates and Jobs that have not succeeded or exhausted their backoff
// limit do not need a Pod list. Otherwise, the uncached reader checks for Pods
// that are not reflected in the Job status. It returns true with an error when
// the Pod state cannot be verified.
func JobShouldWaitForCompletion(
	ctx context.Context,
	apiReader client.Reader,
	job *batchv1.Job,
	deferred bool,
) (bool, error) {
	if deferred {
		return true, nil
	}
	if job == nil {
		return false, nil
	}

	if job.Status.Succeeded == 0 &&
		(job.Spec.BackoffLimit == nil || job.Status.Failed < *job.Spec.BackoffLimit) {
		return true, nil
	}

	return JobHasActiveOrTerminatingPods(ctx, apiReader, job)
}

// CreateOrUpdateDeleteOnImmutableErr deletes an object if an update causes an immutable error
// and returns an error (or an error from the delete if the delete fails). Jobs with active or
// terminating Pods are preserved until those Pods complete; in that case deferred is true and the
// caller should return InProgress so the Job is checked again on a later reconcile.
func CreateOrUpdateDeleteOnImmutableErr(ctx context.Context, k8sClient client.Client, apiReader client.Reader,
	obj client.Object, log logr.Logger, f ctrlutil.MutateFn) (ctrlutil.OperationResult, bool, error) {
	op, err := ctrlutil.CreateOrUpdate(ctx, k8sClient, obj, f)

	// Check if we got an error trying to update an immutable field
	if err != nil && kerrors.IsInvalid(err) && strings.Contains(strings.ToLower(err.Error()), "field is immutable") {
		if job, ok := obj.(*batchv1.Job); ok {
			hasActiveOrTerminatingPods, podErr := JobHasActiveOrTerminatingPods(ctx, apiReader, job)
			if podErr != nil {
				return op, false, podErr
			}
			if hasActiveOrTerminatingPods {
				terminating := int32(0)
				if job.Status.Terminating != nil {
					terminating = *job.Status.Terminating
				}
				log.Info("Deferring immutable Job replacement; nonterminal Pods were found",
					"reportedStatusActive", job.Status.Active, "reportedStatusTerminating", terminating)
				return ctrlutil.OperationResultNone, true, nil
			}
		}

		log.Error(err, "Immutable error updating the object. Will delete so it can be recreated")

		delErr := k8sClient.Delete(ctx, obj, client.PropagationPolicy(metav1.DeletePropagationBackground))
		if delErr != nil {
			return op, false, delErr
		}

		return op, false, fmt.Errorf("unable to update object. Deleting object so it can be recreated")
	}

	return op, false, err
}
