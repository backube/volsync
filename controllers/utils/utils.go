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
	"os"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/reference"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Define the error messages to be returned by VolSync.
const (
	ErrUnableToSetControllerRef = "unable to set controller reference"
)

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

// GetServiceAddress Returns the address of the given service as a string.
func GetServiceAddress(svc *corev1.Service) string {
	address := svc.Spec.ClusterIP
	if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
		if len(svc.Status.LoadBalancer.Ingress) > 0 {
			if svc.Status.LoadBalancer.Ingress[0].Hostname != "" {
				address = svc.Status.LoadBalancer.Ingress[0].Hostname
			} else if svc.Status.LoadBalancer.Ingress[0].IP != "" {
				address = svc.Status.LoadBalancer.Ingress[0].IP
			}
		} else {
			address = ""
		}
	}
	return address
}

func PvcIsReadOnly(pvc *corev1.PersistentVolumeClaim) bool {
	pvcAccessModes := pvc.Status.AccessModes
	if len(pvcAccessModes) == 0 {
		// fall back to spec
		pvcAccessModes = pvc.Spec.AccessModes
	}

	if len(pvcAccessModes) == 1 && pvcAccessModes[0] == corev1.ReadOnlyMany {
		// PVC only supports ROX
		return true
	}

	// All other access modes support write
	return false
}

func AppendEnvVarsForClusterWideProxy(envVars []corev1.EnvVar) []corev1.EnvVar {
	httpProxy, ok := os.LookupEnv("HTTP_PROXY")
	if ok {
		envVars = append(envVars, corev1.EnvVar{Name: "HTTP_PROXY", Value: httpProxy})
		envVars = append(envVars, corev1.EnvVar{Name: "http_proxy", Value: httpProxy})
	}

	httpsProxy, ok := os.LookupEnv("HTTPS_PROXY")
	if ok {
		envVars = append(envVars, corev1.EnvVar{Name: "HTTPS_PROXY", Value: httpsProxy})
		envVars = append(envVars, corev1.EnvVar{Name: "https_proxy", Value: httpsProxy})
	}

	noProxy, ok := os.LookupEnv("NO_PROXY")
	if ok {
		envVars = append(envVars, corev1.EnvVar{Name: "NO_PROXY", Value: noProxy})
		envVars = append(envVars, corev1.EnvVar{Name: "no_proxy", Value: noProxy})
	}

	return envVars
}
