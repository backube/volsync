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

// If an update causes an immutable error, delete the object and return an error (or potentially
// an error from the delete if the delete fails).
// The caller should ensure (usually via a requeue) that createOrUpdate is called on the resource again in
// order for it to be recreated.
func CreateOrUpdateDeleteOnImmutableErr(ctx context.Context, k8sClient client.Client, obj client.Object,
	log logr.Logger, f ctrlutil.MutateFn) (ctrlutil.OperationResult, error) {
	op, err := ctrlutil.CreateOrUpdate(ctx, k8sClient, obj, f)

	// Check if we got an error trying to update an immutable field
	if err != nil && kerrors.IsInvalid(err) && strings.Contains(strings.ToLower(err.Error()), "field is immutable") {
		log.Error(err, "Immutable error updating the object. Will delete so it can be recreated")

		delErr := k8sClient.Delete(ctx, obj, client.PropagationPolicy(metav1.DeletePropagationBackground))
		if delErr != nil {
			return op, delErr
		}

		return op, fmt.Errorf("unable to update object. Deleting object so it can be recreated")
	}

	return op, err
}
