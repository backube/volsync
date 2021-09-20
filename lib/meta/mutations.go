package meta

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type MutationType string

const (
	MutationTypeMerge   = "merge"
	MutationTypeReplace = "replace"
)

// podmutation
type podmutation struct {
	t MutationType
	p *corev1.PodSpec
}

// containermutation
type containermutation struct {
	t MutationType
	c *corev1.Container
}

// metamutation
type metamutation struct {
	t MutationType
	m *metav1.ObjectMeta
}

func (p *podmutation) Type() MutationType {
	return p.t
}

func (p *podmutation) PodSecurityContext() *corev1.PodSecurityContext {
	if p.p == nil {
		return nil
	}
	return p.p.SecurityContext
}

func (p *podmutation) NodeSelector() map[string]string {
	if p.p == nil {
		return nil
	}
	return p.p.NodeSelector
}

func (p *podmutation) NodeName() *string {
	if p.p == nil {
		return nil
	}
	return &p.p.NodeName
}

func (c *containermutation) Type() MutationType {
	return c.t
}

func (c *containermutation) SecurityContext() *corev1.SecurityContext {
	if c.c == nil {
		return nil
	}
	return c.c.SecurityContext
}

func (c *containermutation) Resources() *corev1.ResourceRequirements {
	if c.c == nil {
		return nil
	}
	return &c.c.Resources
}

func (c *containermutation) Name() *string {
	if c.c == nil {
		return nil
	}
	return &c.c.Name
}

func (m *metamutation) Type() MutationType {
	return m.t
}

func (m *metamutation) Labels() map[string]string {
	if m.m == nil {
		return nil
	}
	return m.m.Labels
}

func (m *metamutation) Annotations() map[string]string {
	if m.m == nil {
		return nil
	}
	return m.m.Annotations
}

func (m *metamutation) Name() *string {
	if m.m == nil {
		return nil
	}
	return &m.m.Name
}

func (m *metamutation) OwnerReferences() []metav1.OwnerReference {
	if m.m == nil {
		return nil
	}
	return m.m.OwnerReferences
}

func NewPodSpecMutation(spec *corev1.PodSpec, typ MutationType) PodSpecMutation {
	return &podmutation{
		t: typ,
		p: spec,
	}
}

func NewObjectMetaMutation(objectMeta *metav1.ObjectMeta, typ MutationType) (ObjectMetaMutation, error) {
	if objectMeta.Labels != nil {
		err := ValidateLabels(objectMeta.Labels)
		if err != nil {
			return nil, err
		}
	}
	return &metamutation{
		t: typ,
		m: objectMeta,
	}, nil
}

func NewContainerMutation(spec *corev1.Container, typ MutationType) ContainerMutation {
	return &containermutation{
		t: typ,
		c: spec,
	}
}
