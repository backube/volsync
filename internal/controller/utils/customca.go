package utils

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

// CustomCAObject will generate the appropriate volumesource
// for attaching to a podspec from a source customca secret or configmap.
// Use ValidateCustomCA() to validate a CustomCASpec and return a CustomCAObject.
type CustomCAObject interface {
	// path should be the relative path to filename (key contents will get projected here)
	GetVolumeSource(path string) corev1.VolumeSource
}

type CustomCAObjectSecret struct {
	secret *corev1.Secret
	key    string
}

type CustomCAObjectConfigMap struct {
	configMap *corev1.ConfigMap
	key       string
}

var _ CustomCAObject = &CustomCAObjectSecret{}
var _ CustomCAObject = &CustomCAObjectConfigMap{}

func (c *CustomCAObjectSecret) GetVolumeSource(path string) corev1.VolumeSource {
	return corev1.VolumeSource{
		Secret: &corev1.SecretVolumeSource{
			SecretName: c.secret.Name,
			Items: []corev1.KeyToPath{
				{Key: c.key, Path: path},
			},
		},
	}
}

func (c *CustomCAObjectConfigMap) GetVolumeSource(path string) corev1.VolumeSource {
	return corev1.VolumeSource{
		ConfigMap: &corev1.ConfigMapVolumeSource{
			LocalObjectReference: corev1.LocalObjectReference{Name: c.configMap.Name},
			Items: []corev1.KeyToPath{
				{Key: c.key, Path: path},
			},
		},
	}
}

func ValidateCustomCA(ctx context.Context, cl client.Client, l logr.Logger,
	namespace string, customCA volsyncv1alpha1.CustomCASpec) (CustomCAObject, error) {
	if customCA.Key == "" {
		// Not using a custom CA, no key supplied
		return nil, nil
	}

	if customCA.SecretName != "" {
		// Custom CA is in a secret
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      customCA.SecretName,
				Namespace: namespace,
			},
		}
		logger := l.WithValues("caSecret", client.ObjectKeyFromObject(secret))
		if err := GetAndValidateSecret(ctx, cl, logger, secret, customCA.Key); err != nil {
			logger.Error(err, "Custom CA secret does not contain the proper field", "missingField", customCA.Key)
			return nil, err
		}

		return &CustomCAObjectSecret{secret, customCA.Key}, nil
	} else if customCA.ConfigMapName != "" {
		// Custom CA is in a configmap
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      customCA.ConfigMapName,
				Namespace: namespace,
			},
		}
		logger := l.WithValues("caConfigMap", client.ObjectKeyFromObject(cm))

		if err := GetAndValidateConfigMap(ctx, cl, logger, cm, customCA.Key); err != nil {
			logger.Error(err, "Custom CA configmap does not contain the proper field", "missingField", customCA.Key)
			return nil, err
		}

		return &CustomCAObjectConfigMap{cm, customCA.Key}, nil
	}

	// Not using a customCA, no secretName/configMapName supplied
	return nil, nil
}
