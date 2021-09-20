package meta

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// TODO: alpatel,shurley: consider if this can be a json patch based implementation

// NamespacedNamePair knows how to map a source and a destination namespace involved in state transfer
type NamespacedNamePair interface {
	// Source represents source namespace and name
	Source() types.NamespacedName
	// Destination represents destination namespace and name
	Destination() types.NamespacedName
}

// Mutation allows mutating state transfer resources on the fly based on the type
type Mutation interface {
	Type() MutationType
}

// PodSpecMutation knows how to mutate PodSpec fields of a corev1.Pod resource
type PodSpecMutation interface {
	Mutation
	// SecurityContext returns a PodSecurityContext for the target Pod
	PodSecurityContext() *corev1.PodSecurityContext
	// NodeSelector returns a node selector for the target Pod
	NodeSelector() map[string]string
	// NodeName returns a node name for the target Pod
	NodeName() *string
}

type ContainerMutation interface {
	Mutation
	// Name returns a name for the container
	Name() *string
	// SecurityContext returns mutated security context for the target container
	SecurityContext() *corev1.SecurityContext
	// Resources returns mutated resources on the container
	Resources() *corev1.ResourceRequirements
}

// ObjectMetaMutation knows how to mutate fields of a metav1.ObjectMeta resource
type ObjectMetaMutation interface {
	Mutation
	// Labels returns a set of labels for the target ObjectMeta
	Labels() map[string]string
	// Annotations returns a set of annotations for the target ObjectMeta
	Annotations() map[string]string
	// Name returns a name for the target ObjectMeta
	Name() *string
	// OwnerReferences returns a list of OwnerReferences
	OwnerReferences() []metav1.OwnerReference
}
