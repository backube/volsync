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

package mover

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	VolSyncPrefix = "volsync-"

	// MoverTerminationGracePeriodSeconds is the terminationGracePeriodSeconds
	// used for all Job-based mover Pods (Restic, rclone, rsync, rsync-TLS).
	//
	// Kubernetes defaults to 30s when the field is unset. Restic handles
	// SIGTERM and bounds its lock-cleanup work with restic.UnlockCancelDelay
	// (60s), so a grace period of 120s gives the forwarded stop signal time to
	// release the repository lock plus a margin. The mover entrypoint scripts
	// forward TERM to their child processes (see mover-common/shutdown.sh),
	// which is what lets the cleanup run at all.
	MoverTerminationGracePeriodSeconds int64 = 120
)

// Mover is a common interface that all data movers implement
type Mover interface {
	// The name of this data mover
	Name() string

	// Synchronize begins or continues a synchronization attempt. Attempts will
	// continue at least until the Result indicates that the synchronization is
	// complete. Must be idempotent.
	Synchronize(ctx context.Context) (Result, error)

	// Cleanup begins or continues the post-synchronization cleanup of temporary
	// resources. Must be idempotent.
	Cleanup(ctx context.Context) (Result, error)
}

// Result indicates the outcome of a synchronization attempt
type Result struct {
	// Completed is set to true if the synchronization has completed. RetryAfter
	// will be ignored.
	Completed bool

	// Image is the resulting data image (PVC or Snapshot) that has been created
	// by the Synchronize() operation.
	Image *corev1.TypedLocalObjectReference

	// RetryAfter is used to indicate whether synchronization should be
	// explicitly retried, and when. Setting to nil (default) does not cause an
	// explicit retry, but Synchronize() will be retried when a watched object
	// is modified. Setting to 0 or a negative value indicates an immediate
	// retry. Positive values provide a delay.
	RetryAfter *time.Duration
}

// ReconcileResult converts a Result into controllerruntime's reconcile result
// structure
func (mr Result) ReconcileResult() ctrl.Result {
	if mr.RetryAfter == nil {
		return ctrl.Result{}
	}

	retryAfter := *mr.RetryAfter
	if retryAfter <= 0 {
		retryAfter = time.Nanosecond
	}
	return ctrl.Result{RequeueAfter: retryAfter}
}

// InProgress result indicates that the requested operation is still ongoing,
// but it does not request an explicit requeueing.
func InProgress() Result {
	// When we have an operation in-progress, we should still reconcile
	// periodically so that we can detect if we aren't progressing.
	retryTime := 1 * time.Minute
	return Result{
		RetryAfter: &retryTime,
	}
}

// RetryAfter indicates the operation is ongoing and requests explicit
// requeueing after the provided duration.
func RetryAfter(s time.Duration) Result { return Result{RetryAfter: &s} }

// Complete indicates that the operation has completed.
func Complete() Result {
	return Result{
		Completed: true,
	}
}

// CompleteWithImage indicates that the operation has completed, and it provides
// the synchronized image to the controller.
func CompleteWithImage(image *corev1.TypedLocalObjectReference) Result {
	return Result{
		Completed: true,
		Image:     image,
	}
}
