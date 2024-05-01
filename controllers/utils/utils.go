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
	"sort"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/reference"
	"sigs.k8s.io/controller-runtime/pkg/client"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch

// Define the error messages to be returned by VolSync.
const (
	ErrUnableToSetControllerRef = "unable to set controller reference"
)

// Check if error is due to the CRD not being present (API kind/group not available)
// This has changed recently in controller-runtime v0.15.0, see:
// https://github.com/kubernetes-sigs/controller-runtime/pull/2116
func IsCRDNotPresentError(err error) bool {
	if apimeta.IsNoMatchError(err) || kerrors.IsNotFound(err) ||
		strings.Contains(err.Error(), "failed to get API group resources") {
		return true
	}
	return false
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

func GetAndValidateConfigMap(ctx context.Context, cl client.Client,
	logger logr.Logger, configMap *corev1.ConfigMap, fields ...string) error {
	if err := cl.Get(ctx, client.ObjectKeyFromObject(configMap), configMap); err != nil {
		logger.Error(err, "failed to get ConfigMap with provided name", "ConfigMap", client.ObjectKeyFromObject(configMap))
		return err
	}
	if err := ConfigMapHasFields(configMap, fields...); err != nil {
		logger.Error(err, "configMap does not contain the proper fields", "Secret", client.ObjectKeyFromObject(configMap))
		return err
	}
	return nil
}

func ConfigMapHasFields(configMap *corev1.ConfigMap, fields ...string) error {
	data := configMap.Data
	if data == nil || len(data) < len(fields) {
		return fmt.Errorf("configmap should have fields: %v", fields)
	}
	for _, k := range fields {
		if _, found := data[k]; !found {
			return fmt.Errorf("configmap is missing field: %v", k)
		}
	}
	return nil
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

func PvcIsBlockMode(pvc *corev1.PersistentVolumeClaim) bool {
	return pvc.Spec.VolumeMode != nil && *pvc.Spec.VolumeMode == corev1.PersistentVolumeBlock
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

// Append k/v from the secret if they start with RCLONE_
func AppendRCloneEnvVars(secret *corev1.Secret, envVars []corev1.EnvVar) []corev1.EnvVar {
	rcloneKeys := []string{}
	for key := range secret.Data {
		if strings.HasPrefix(key, "RCLONE_") {
			rcloneKeys = append(rcloneKeys, key)
		}
	}

	if len(rcloneKeys) > 0 {
		// Sort so we do not keep re-ordering env vars (range over map keys will give us inconsistent sorting)
		sort.Strings(rcloneKeys)

		for _, sortedKey := range rcloneKeys {
			envVars = append(envVars, EnvFromSecret(secret.Name, sortedKey, true))
		}
	}

	return envVars
}

// Will append the MoverDebug Env var if the volsyncv1alpha1.EnableDebugMoverAnnotation
// annotation is set on the corresponding ReplicationSource or Destination
func AppendDebugMoverEnvVar(replicationSourceOrDestObj metav1.Object, envVars []corev1.EnvVar) []corev1.EnvVar {
	// If the annotation exists on the RS/RD (with any value) then we assume mover debug mode is desired
	_, debugMoverEnabled := replicationSourceOrDestObj.GetAnnotations()[volsyncv1alpha1.EnableDebugMoverAnnotation]
	if debugMoverEnabled {
		envVars = append(envVars, corev1.EnvVar{Name: "DEBUG_MOVER", Value: "1"})
	}

	return envVars
}

// Updates to set the securityContext, podLabels on mover pod in the spec and resourceRequirements on the mover
// containers based on what is set in the MoverConfig
func UpdatePodTemplateSpecFromMoverConfig(podTemplateSpec *corev1.PodTemplateSpec,
	moverConfig volsyncv1alpha1.MoverConfig, defaultMoverResources corev1.ResourceRequirements) {
	if podTemplateSpec == nil {
		return
	}

	// Security context (nil by default)
	podTemplateSpec.Spec.SecurityContext = moverConfig.MoverSecurityContext

	// Adjust the job/deploy containers resourceRequirements based on resourceRequirements from the moverConfig
	moverResources := defaultMoverResources
	if moverConfig.MoverResources != nil {
		moverResources = *moverConfig.MoverResources
	}
	for i := range podTemplateSpec.Spec.Containers {
		podTemplateSpec.Spec.Containers[i].Resources = moverResources
	}

	// Set custom labels on the job pod if specified in the moverConfig
	if podTemplateSpec.Labels == nil {
		podTemplateSpec.Labels = map[string]string{}
	}
	for label, value := range moverConfig.MoverPodLabels {
		podTemplateSpec.Labels[label] = value
	}
}
