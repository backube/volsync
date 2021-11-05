package meta

import (
	apivalidation "k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metav1validation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

type Labels map[string]string

func (l Labels) ApplyTo(objMeta *metav1.ObjectMeta) error {
	errList := metav1validation.ValidateLabels(l, field.NewPath("metadata", "labels"))
	if errList != nil {
		return errList.ToAggregate()
	}
	objMeta.Labels = l
	return nil
}

type Annotations map[string]string

func (a Annotations) ApplyTo(objMeta *metav1.ObjectMeta) error {
	errList := apivalidation.ValidateAnnotations(a, field.NewPath("metadata", "annotation"))
	if errList != nil {
		return errList.ToAggregate()
	}
	objMeta.Annotations = a
	return nil
}

type OwnerReference metav1.OwnerReference

func (o OwnerReference) ApplyTo(objMeta *metav1.ObjectMeta) error {
	errList := apivalidation.ValidateOwnerReferences([]metav1.OwnerReference{metav1.OwnerReference(o)}, field.NewPath("metadata", "ownerReferences"))
	if errList != nil {
		return errList.ToAggregate()
	}
	objMeta.OwnerReferences = []metav1.OwnerReference{metav1.OwnerReference(o)}
	return nil
}

type Finalizers []string

func (f Finalizers) ApplyTo(objMeta *metav1.ObjectMeta) error {
	errList := apivalidation.ValidateFinalizers(f, field.NewPath("metadata", "finalizers"))
	if errList != nil {
		return errList.ToAggregate()
	}
	objMeta.Finalizers = f
	return nil
}

func GetMetaObjectWithMutations(namespacedName types.NamespacedName, metaMutations []ObjectMetaMutation) (*metav1.ObjectMeta, error) {
	m := &metav1.ObjectMeta{
		Name:      namespacedName.Name,
		Namespace: namespacedName.Namespace,
	}

	for _, opt := range metaMutations {
		err := opt.ApplyTo(m)
		if err != nil {
			return nil, err
		}
	}
	return m, nil
}
