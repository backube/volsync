package cmd

import (
	"fmt"
	"strings"

	scribev1alpha1 "github.com/backube/scribe/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

type ReplicationOptions struct {
	CopyMethod              string //v1alpha1.CopyMethodType
	Capacity                string //*resource.Quantity
	StorageClassName        string
	AccessMode              string //[]corev1.PersistentVolumeAccessMode
	Address                 string
	VolumeSnapshotClassName string
	PVC                     string
	SSHUser                 string
	ServiceType             string //*corev1.ServiceType
	Port                    int32  //int32
	Path                    string
	RcloneConfig            string
	Provider                string
	ProviderParameters      string //map[string]string
}

type CommonOptions struct {
	CopyMethod          scribev1alpha1.CopyMethodType
	Capacity            *resource.Quantity
	StorageClassName    *string
	AccessModes         []corev1.PersistentVolumeAccessMode
	Address             *string
	VolumeSnapClassName *string
	PVC                 *string
	SSHUser             *string
	ServiceType         corev1.ServiceType
	Port                *int32
	Path                *string
	Parameters          map[string]string
}

func (o *ReplicationOptions) GetCommonOptions() (*CommonOptions, error) {
	c := &CommonOptions{}
	var ok bool
	c = o.getCapacity(c)
	if c, ok = o.getCopyMethod(c); !ok {
		return nil, fmt.Errorf("unrecognized --dest-copy-method %s", o.CopyMethod)
	}
	if c, ok = o.getAccessModes(c); !ok {
		return nil, fmt.Errorf("unrecognized --dest-access-modes %s", o.CopyMethod)
	}
	if c, ok = o.getServiceType(c); !ok {
		return nil, fmt.Errorf("unrecognized --dest-service-type %s", o.ServiceType)
	}
	if o.Port == 0 {
		c.Port = nil
	}
	c.Address = getOption(o.Address)
	c.SSHUser = getOption(o.SSHUser)
	c.Path = getOption(o.Path)
	c.StorageClassName = getOption(o.StorageClassName)
	c.VolumeSnapClassName = getOption(o.VolumeSnapshotClassName)
	c.PVC = getOption(o.PVC)
	c.Parameters = make(map[string]string)
	if len(o.ProviderParameters) > 0 {
		p := strings.Split(o.ProviderParameters, ",")
		for _, kv := range p {
			pair := strings.Split(kv, "=")
			if len(pair) != 2 {
				return nil, fmt.Errorf("error parsing --provider-parameters %s, pass key=value,key1=value1", o.ProviderParameters)
			}
			c.Parameters[pair[0]] = pair[1]
		}
	}
	return c, nil
}

func getOption(opt string) *string {
	if len(opt) > 0 {
		return &opt
	}
	return nil
}

func (o *ReplicationOptions) getCopyMethod(c *CommonOptions) (*CommonOptions, bool) {
	// CopyMethod is always required
	switch strings.ToLower(o.CopyMethod) {
	case "none":
		c.CopyMethod = scribev1alpha1.CopyMethodNone
		return c, true
	case "clone":
		c.CopyMethod = scribev1alpha1.CopyMethodClone
		return c, true
	case "snapshot":
		c.CopyMethod = scribev1alpha1.CopyMethodSnapshot
		return c, true
	}
	return nil, false
}

func (o *ReplicationOptions) getCapacity(c *CommonOptions) *CommonOptions {
	// Capacity not always required
	switch {
	case len(o.Capacity) > 0:
		capacity := resource.MustParse(o.Capacity)
		c.Capacity = &capacity
		return c
	}
	return c
}

func (o *ReplicationOptions) getAccessModes(c *CommonOptions) (*CommonOptions, bool) {
	// AccessMode not always required
	if len(o.AccessMode) > 0 {
		switch o.AccessMode {
		case "ReadWriteOnce":
			c.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
		case "ReadWriteMany":
			c.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}
		case "ReadOnlyMany":
			c.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadOnlyMany}
		default:
			return nil, false
		}
	}
	return c, true
}

func (o *ReplicationOptions) getServiceType(c *CommonOptions) (*CommonOptions, bool) {
	// defaults to ClusterIP if not set
	switch {
	case len(o.ServiceType) > 0:
		switch strings.ToLower(o.ServiceType) {
		case "clusterip":
			c.ServiceType = corev1.ServiceTypeClusterIP
			return c, true
		case "loadbalancer":
			c.ServiceType = corev1.ServiceTypeLoadBalancer
			return c, true
		default:
			return nil, false
		}
	default:
		c.ServiceType = corev1.ServiceTypeClusterIP
	}
	return c, true
}
