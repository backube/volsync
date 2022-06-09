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
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/reference"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	VolsyncCreatedByLabelKey   = "app.kubernetes.io/created-by"
	VolsyncCreatedByLabelValue = "volsync"
)

// Add common label(s) to object created by VolSync
func AddVolSyncLabels(obj metav1.Object) {
	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	// Label to indicate the object is created by VolSync
	labels[VolsyncCreatedByLabelKey] = VolsyncCreatedByLabelValue
	obj.SetLabels(labels)
}

func AddControllerReferenceAndVolSyncLabels(owner metav1.Object, obj metav1.Object,
	scheme *runtime.Scheme, logger logr.Logger) error {
	if err := ctrl.SetControllerReference(owner, obj, scheme); err != nil {
		logger.Error(err, "unable to set controller reference")
		return err
	}

	AddVolSyncLabels(obj)

	return nil
}

func GetAndValidateSecret(ctx context.Context, cl client.Client,
	logger logr.Logger, secret *corev1.Secret, fields ...string) error {
	if err := cl.Get(ctx, client.ObjectKeyFromObject(secret), secret); err != nil {
		logger.Error(err, "failed to get Secret with provided name", "Secret", client.ObjectKeyFromObject(secret))
		return err
	}
	if err := SecretHasFields(secret, fields...); err != nil {
		logger.Error(err, "secret does not contain the proper fields", "Secret", client.ObjectKeyFromObject(secret))
		return err
	}
	return nil
}

func SecretHasFields(secret *corev1.Secret, fields ...string) error {
	data := secret.Data
	if data == nil || len(data) < len(fields) {
		return fmt.Errorf("secret should have fields: %v", fields)
	}
	for _, k := range fields {
		if _, found := data[k]; !found {
			return fmt.Errorf("secret is missing field: %v", k)
		}
	}
	return nil
}

func EnvFromSecret(secretName string, field string, optional bool) corev1.EnvVar {
	return corev1.EnvVar{
		Name: field,
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secretName,
				},
				Key:      field,
				Optional: &optional,
			},
		},
	}
}

func KindAndName(scheme *runtime.Scheme, obj client.Object) string {
	ref, err := reference.GetReference(scheme, obj)
	if err != nil {
		return obj.GetName()
	}
	return ref.Kind + "/" + ref.Name
}
