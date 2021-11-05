package meta

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ObjectMetaMutation knows how to mutate fields of a metav1.ObjectMeta resource
type ObjectMetaMutation interface {
	ApplyTo(*metav1.ObjectMeta) error
}
