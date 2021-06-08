package cmd

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	scribev1alpha1 "github.com/backube/scribe/api/v1alpha1"
)

const (
	scribeSource = "source"
	scribeDest   = "dest"
)

type sharedOptions struct {
	CopyMethod              string //v1alpha1.CopyMethodType
	Capacity                string //*resource.Quantity
	StorageClass            string
	AccessMode              string //[]corev1.PersistentVolumeAccessMode
	VolumeSnapshotClassName string
	SSHUser                 string
	ServiceType             string //*corev1.ServiceType
	Port                    int32  //int32
	RcloneConfig            string
	Provider                string
	ProviderParameters      string //map[string]string
}

//nolint:funlen
func (o *SetupReplicationOptions) getCommonOptions(c *sharedOptions, mode string) error {
	if err := o.getCopyMethod(c.CopyMethod, mode); err != nil {
		return err
	}
	if err := o.getAccessModes(c.AccessMode, mode); err != nil {
		return err
	}
	if err := o.getServiceType(c.ServiceType, mode); err != nil {
		return err
	}
	port := &c.Port
	if c.Port == 0 {
		port = nil
	}
	if err := o.getCapacity(c.Capacity, mode); err != nil {
		return err
	}
	switch mode {
	case scribeDest:
		o.RepOpts.Dest.Port = port
		o.RepOpts.Dest.SSHUser = getOption(c.SSHUser)
		o.RepOpts.Dest.StorageClass = getOption(c.StorageClass)
		o.RepOpts.Dest.VolumeSnapClassName = getOption(c.VolumeSnapshotClassName)
		o.RepOpts.Dest.Provider = c.Provider
		o.RepOpts.Dest.Parameters = make(map[string]string)
		if len(c.ProviderParameters) > 0 {
			p := strings.Split(c.ProviderParameters, ",")
			for _, kv := range p {
				pair := strings.Split(kv, "=")
				if len(pair) != 2 {
					return fmt.Errorf("error parsing --provider-parameters %s, pass key=value,key1=value1", c.ProviderParameters)
				}
				o.RepOpts.Dest.Parameters[pair[0]] = pair[1]
			}
		}
	case scribeSource:
		o.RepOpts.Source.Port = port
		o.RepOpts.Source.SSHUser = getOption(c.SSHUser)
		o.RepOpts.Source.StorageClass = getOption(c.StorageClass)
		o.RepOpts.Source.VolumeSnapClassName = getOption(c.VolumeSnapshotClassName)
		o.RepOpts.Source.Provider = c.Provider
		o.RepOpts.Source.Parameters = make(map[string]string)
		if len(c.ProviderParameters) > 0 {
			p := strings.Split(c.ProviderParameters, ",")
			for _, kv := range p {
				pair := strings.Split(kv, "=")
				if len(pair) != 2 {
					return fmt.Errorf("error parsing --provider-parameters %s, pass key=value,key1=value1", c.ProviderParameters)
				}
				o.RepOpts.Source.Parameters[pair[0]] = pair[1]
			}
		}
	}
	return nil
}

func getOption(opt string) *string {
	if len(opt) > 0 {
		return &opt
	}
	return nil
}

func (o *SetupReplicationOptions) getCapacity(c string, mode string) error {
	// Capacity not always required
	var capacity resource.Quantity
	srcPVC, err := o.GetSourcePVC(context.Background())
	if err != nil {
		return err
	}
	switch mode {
	case scribeDest:
		if len(c) > 0 {
			o.RepOpts.Dest.Capacity = resource.MustParse(c)
		} else {
			capacity = srcPVC.Spec.Resources.Requests[corev1.ResourceStorage]
			o.RepOpts.Dest.Capacity = capacity
		}
	case scribeSource:
		if len(c) > 0 {
			o.RepOpts.Source.Capacity = resource.MustParse(c)
		} else {
			capacity = srcPVC.Spec.Resources.Requests[corev1.ResourceStorage]
			o.RepOpts.Source.Capacity = capacity
		}
	}
	return nil
}

func (o *SetupReplicationOptions) getCopyMethod(c string, mode string) error {
	var cm scribev1alpha1.CopyMethodType
	// CopyMethod is always required
	switch strings.ToLower(c) {
	case "none":
		cm = scribev1alpha1.CopyMethodNone
	case "clone":
		cm = scribev1alpha1.CopyMethodClone
	case "snapshot":
		cm = scribev1alpha1.CopyMethodSnapshot
	default:
		return fmt.Errorf("unsupported %s copyMethod %s", mode, c)
	}
	switch mode {
	case scribeDest:
		o.RepOpts.Dest.CopyMethod = cm
	case scribeSource:
		o.RepOpts.Source.CopyMethod = cm
	}
	return nil
}

func (o *SetupReplicationOptions) getAccessModes(c string, mode string) error {
	var am []corev1.PersistentVolumeAccessMode
	// AccessMode not always required
	if len(c) > 0 {
		switch c {
		case "ReadWriteOnce":
			am = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
		case "ReadWriteMany":
			am = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}
		case "ReadOnlyMany":
			am = []corev1.PersistentVolumeAccessMode{corev1.ReadOnlyMany}
		default:
			return fmt.Errorf("unsupported %s accessMode: %s", mode, c)
		}
	}
	switch mode {
	case scribeDest:
		o.RepOpts.Dest.AccessModes = am
	case scribeSource:
		o.RepOpts.Source.AccessModes = am
	}
	return nil
}

func (o *SetupReplicationOptions) getServiceType(c string, mode string) error {
	// defaults to ClusterIP if not set
	var st corev1.ServiceType
	if len(c) > 0 {
		switch strings.ToLower(c) {
		case "clusterip":
			st = corev1.ServiceTypeClusterIP
		case "loadbalancer":
			st = corev1.ServiceTypeLoadBalancer
		default:
			return fmt.Errorf("unsupported %s serviceType %s", mode, c)
		}
	}
	switch mode {
	case scribeDest:
		o.RepOpts.Dest.ServiceType = st
	case scribeSource:
		o.RepOpts.Source.ServiceType = st
	}
	return nil
}
