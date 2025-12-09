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

//nolint:revive
package utils

import (
	"context"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

func PrivilegedMoversOk(ctx context.Context, cl client.Client, logger logr.Logger,
	namespace string) (bool, error) {
	// Check namespace to see if privileged-mover annotation is there and "true"
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	err := cl.Get(ctx, client.ObjectKeyFromObject(ns), ns)
	if err != nil {
		logger.Error(err, "Error getting namespace", "namespace", namespace)
		return false, err
	}

	privilegedMoverOkVal, ok := ns.GetAnnotations()[volsyncv1alpha1.PrivilegedMoversNamespaceAnnotation]
	if ok && strings.ToLower(privilegedMoverOkVal) == "true" {
		logger.Info("Namespace allows volsync privileged movers",
			"namespace", namespace, "Annotation", volsyncv1alpha1.PrivilegedMoversNamespaceAnnotation,
			"Annotation value", privilegedMoverOkVal)
		return true, nil
	}

	return false, nil
}
