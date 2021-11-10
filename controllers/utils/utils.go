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
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetAndValidateSecret(ctx context.Context, cl client.Client,
	logger logr.Logger, secret *corev1.Secret, fields ...string) error {
	if err := cl.Get(ctx, client.ObjectKeyFromObject(secret), secret); err != nil {
		logger.Error(err, "failed to get Secret with provided name", "Secret", client.ObjectKeyFromObject(secret))
		return err
	}
	if err := secretHasFields(secret, fields...); err != nil {
		logger.Error(err, "secret does not contain the proper fields", "Secret", client.ObjectKeyFromObject(secret))
		return err
	}
	return nil
}

func secretHasFields(secret *corev1.Secret, fields ...string) error {
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
