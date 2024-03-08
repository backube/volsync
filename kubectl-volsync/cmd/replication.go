/*
Copyright © 2021 The VolSync authors

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <http://www.gnu.org/licenses/>.
*/
package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	errorsutil "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/component-base/logs"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

const ReplicationRelationshipType RelationshipType = "replication"

// replicationRelationship holds the config state for replication-type
// relationships
type replicationRelationship struct {
	Relationship
	data replicationRelationshipDataV2
	rh   replicationHandler
}

type replicationHandler interface {
	ApplyDestination(ctx context.Context, c client.Client,
		dstPVC *corev1.PersistentVolumeClaim, addIDLabel func(obj client.Object),
		destConfig *replicationRelationshipDestinationV2) (*string, *corev1.Secret, error)
	ApplySource(ctx context.Context, c client.Client,
		address *string, dstKeys *corev1.Secret, addIDLabel func(obj client.Object),
		sourceConfig *replicationRelationshipSourceV2) error
}

// Old v1 version of the data
type replicationRelationshipData struct {
	// Config file/struct version used so we know how to decode when parsing
	// from disk
	Version int
	// Config info for the source side of the relationship
	Source *replicationRelationshipSource
	// Config info for the destination side of the relationship
	Destination *replicationRelationshipDestination
}

// replicationRelationshipData is the state that will be saved to the
// relationship config file
type replicationRelationshipDataV2 struct {
	// Config file/struct version used so we know how to decode when parsing
	// from disk
	Version int
	// True if the ReplicationDestination should use RsyncTLS
	IsRsyncTLS bool
	// Config info for the source side of the relationship
	Source *replicationRelationshipSourceV2
	// Config info for the destination side of the relationship
	Destination *replicationRelationshipDestinationV2
}

type replicationRelationshipSource struct {
	// Cluster context name
	Cluster string
	// Namespace on source cluster
	Namespace string
	// Name of PVC being replicated
	PVCName string
	// Name of ReplicationSource object
	RSName string
	// Parameters for the ReplicationSource
	Source volsyncv1alpha1.ReplicationSourceRsyncSpec
	// Scheduling parameters
	Trigger volsyncv1alpha1.ReplicationSourceTriggerSpec
}

type replicationRelationshipSourceV2 struct {
	// Cluster context name
	Cluster string
	// Namespace on source cluster
	Namespace string
	// Name of PVC being replicated
	PVCName string
	// Name of ReplicationSource object
	RSName string
	// Parameters for the ReplicationSource volume options
	ReplicationSourceVolumeOptions volsyncv1alpha1.ReplicationSourceVolumeOptions
	// Scheduling parameters
	Trigger volsyncv1alpha1.ReplicationSourceTriggerSpec
}

type replicationRelationshipDestination struct {
	// Cluster context name
	Cluster string
	// Namespace on destination cluster
	Namespace string
	// Name of the ReplicationDestination object
	RDName string
	// Parameters for the ReplicationDestination
	Destination volsyncv1alpha1.ReplicationDestinationRsyncSpec
}

type replicationRelationshipDestinationV2 struct {
	// Cluster context name
	Cluster string
	// Namespace on destination cluster
	Namespace string
	// Name of the ReplicationDestination object
	RDName string
	// Parameters for the ReplicationDestination volume options
	ReplicationDestinationVolumeOptions volsyncv1alpha1.ReplicationDestinationVolumeOptions
	// Service Type for the ReplicationDestination
	ServiceType *corev1.ServiceType
}

// replicationCmd represents the replication command
var replicationCmd = &cobra.Command{
	Use:   "replication",
	Short: i18n.T("Replicate data between two PersistentVolumes"),
	Long: templates.LongDesc(i18n.T(`
	Replicate the contents of one PersistentVolume to another.

	This set of commands is designed to set up and manage a replication
	relationship between two different PVCs in the same Namespace, across
	Namespaces, or in different clusters. The contents of the volume can be
	replicated either on-demand or based on a provided schedule.
	`)),
}

func init() {
	rootCmd.AddCommand(replicationCmd)

	// Add logging flags to all sub-commands
	logs.AddFlags(migrationCmd.PersistentFlags())

	replicationCmd.PersistentFlags().StringP("relationship", "r", "", "relationship name")
	cobra.CheckErr(replicationCmd.MarkPersistentFlagRequired("relationship"))
}

func loadReplicationRelationship(cmd *cobra.Command) (*replicationRelationship, error) {
	r, err := LoadRelationshipFromCommand(cmd, ReplicationRelationshipType)
	if err != nil {
		return nil, err
	}

	rr := &replicationRelationship{
		Relationship: *r,
	}
	// Decode according to the file version
	version := rr.GetInt("data.version")
	switch version {
	case 1:
		// Old version of config, migrate to v2
		datav1 := &replicationRelationshipData{}
		if err := rr.GetData(datav1); err != nil {
			return nil, err
		}
		rr.convertDataToV2(datav1)
	case 2:
		if err := rr.GetData(&rr.data); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported config file version %d", version)
	}

	if rr.data.IsRsyncTLS {
		rr.rh = &replicationHandlerRsyncTLS{}
	} else {
		rr.rh = &replicationHandlerRsync{}
	}

	return rr, nil
}

func (rr *replicationRelationship) Save() error {
	if err := rr.SetData(rr.data); err != nil {
		return err
	}
	// resource.Quantity doesn't properly encode, so we need to do it manually
	if rr.data.Source != nil && rr.data.Source.ReplicationSourceVolumeOptions.Capacity != nil {
		rr.Set("data.source.replicationsourcevolumeoptions.capacity",
			rr.data.Source.ReplicationSourceVolumeOptions.Capacity.String())
	}
	if rr.data.Destination != nil && rr.data.Destination.ReplicationDestinationVolumeOptions.Capacity != nil {
		rr.Set("data.destination.replicationdestinationvolumeoptions.capacity",
			rr.data.Destination.ReplicationDestinationVolumeOptions.Capacity.String())
	}
	return rr.Relationship.Save()
}

func (rr *replicationRelationship) convertDataToV2(datav1 *replicationRelationshipData) {
	rr.data = replicationRelationshipDataV2{
		Version:    2,
		IsRsyncTLS: false, // Rsync-TLS support wasn't there in v1
		Source: &replicationRelationshipSourceV2{
			Cluster:                        datav1.Source.Cluster,
			Namespace:                      datav1.Source.Namespace,
			PVCName:                        datav1.Source.PVCName,
			RSName:                         datav1.Source.RSName,
			ReplicationSourceVolumeOptions: datav1.Source.Source.ReplicationSourceVolumeOptions,
			Trigger:                        datav1.Source.Trigger,
		},
		Destination: &replicationRelationshipDestinationV2{
			Cluster:                             datav1.Destination.Cluster,
			Namespace:                           datav1.Destination.Namespace,
			RDName:                              datav1.Destination.RDName,
			ReplicationDestinationVolumeOptions: datav1.Destination.Destination.ReplicationDestinationVolumeOptions,
			ServiceType:                         datav1.Destination.Destination.ServiceType,
		},
	}
}

// GetClients returns clients to access the src & dst clusters (srcClient,
// dstClient, error)
func (rr *replicationRelationship) GetClients() (client.Client, client.Client, error) {
	var srcClient, dstClient client.Client
	var err error
	errList := []error{}
	if rr.data.Source != nil {
		if srcClient, err = newClient(rr.data.Source.Cluster); err != nil {
			klog.Errorf("unable to create client for source cluster: %v", err)
			errList = append(errList, err)
		}
	}
	if rr.data.Destination != nil {
		if dstClient, err = newClient(rr.data.Destination.Cluster); err != nil {
			klog.Errorf("unable to create client for destination cluster: %v", err)
			errList = append(errList, err)
		}
	}
	return srcClient, dstClient, errorsutil.NewAggregate(errList)
}

// DeleteSource removes the resources we've created on the source cluster
func (rr *replicationRelationship) DeleteSource(ctx context.Context,
	srcClient client.Client) error {
	src := rr.data.Source
	if srcClient == nil || src == nil {
		// Nothing to do because we don't have a client or the source isn't
		// defined
		return nil
	}

	errList := []error{}
	for _, o := range []client.Object{
		// cleaning up requires deleting both RS and the Secret we copied
		&volsyncv1alpha1.ReplicationSource{},
		&corev1.Secret{},
	} {
		err := srcClient.DeleteAllOf(ctx, o,
			client.InNamespace(src.Namespace),
			client.MatchingLabels{RelationshipLabelKey: rr.ID().String()},
			client.PropagationPolicy(metav1.DeletePropagationBackground))
		if client.IgnoreNotFound(err) != nil {
			klog.Errorf("unable to remove previous Source objects: %v", err)
			errList = append(errList, err)
		}
	}
	return errorsutil.NewAggregate(errList)
}

// DeleteDestination removes the resources we've created on the destination
// cluster
func (rr *replicationRelationship) DeleteDestination(ctx context.Context,
	dstClient client.Client) error {
	dst := rr.data.Destination
	if dstClient == nil || dst == nil {
		// Nothing to do because we don't have a client or the destination isn't
		// defined
		return nil
	}

	err := dstClient.DeleteAllOf(ctx, &volsyncv1alpha1.ReplicationDestination{},
		client.InNamespace(dst.Namespace),
		client.MatchingLabels{RelationshipLabelKey: rr.ID().String()},
		client.PropagationPolicy(metav1.DeletePropagationBackground))
	err = client.IgnoreNotFound(err)
	if err != nil {
		klog.Errorf("unable to remove previous Destination objects: %v", err)
	}
	return err
}

func (rr *replicationRelationship) Apply(ctx context.Context, srcClient client.Client,
	dstClient client.Client) error {
	if rr.data.Source == nil {
		return fmt.Errorf("please define a replication source with \"set-source\"")
	}
	if rr.data.Destination == nil {
		return fmt.Errorf("please define a replication destination with \"set-destination\"")
	}

	// Get Source PVC info
	srcPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rr.data.Source.PVCName,
			Namespace: rr.data.Source.Namespace,
		},
	}
	if err := srcClient.Get(ctx, client.ObjectKeyFromObject(srcPVC), srcPVC); err != nil {
		return fmt.Errorf("unable to retrieve source PVC: %w", err)
	}

	var dstPVC *corev1.PersistentVolumeClaim
	if rr.data.Destination.ReplicationDestinationVolumeOptions.CopyMethod == volsyncv1alpha1.CopyMethodSnapshot {
		// We need to ensure the RD has defaults based on the source volume
		if rr.data.Destination.ReplicationDestinationVolumeOptions.Capacity == nil {
			capacity := srcPVC.Spec.Resources.Requests[corev1.ResourceStorage]
			rr.data.Destination.ReplicationDestinationVolumeOptions.Capacity = &capacity
		}
		if len(rr.data.Destination.ReplicationDestinationVolumeOptions.AccessModes) == 0 {
			rr.data.Destination.ReplicationDestinationVolumeOptions.AccessModes = srcPVC.Spec.AccessModes
		}
	} else {
		// Since we're not snapshotting on the dest, we need to ensure there's a
		// PVC with the final name present on the cluster
		var err error
		dstPVC, err = rr.ensureDestinationPVC(ctx, dstClient, srcPVC)
		if err != nil {
			return fmt.Errorf("unable to create PVC on destination: %w", err)
		}
	}

	address, secret, err := rr.rh.ApplyDestination(ctx, dstClient, dstPVC, rr.AddIDLabel, rr.data.Destination)
	if err != nil {
		return err
	}

	return rr.rh.ApplySource(ctx, srcClient, address, secret, rr.AddIDLabel, rr.data.Source)
}

// Gets or creates the destination PVC
func (rr *replicationRelationship) ensureDestinationPVC(ctx context.Context, c client.Client,
	srcPVC *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	// By default, we can guess AccessModes and Capacity from the source PVC,
	// then allow that to be overridden.
	var accessModes []corev1.PersistentVolumeAccessMode
	var capacity resource.Quantity
	if srcPVC != nil { // By default, we duplicate what's on the source
		accessModes = srcPVC.Spec.AccessModes
		capacity = srcPVC.Spec.Resources.Requests[corev1.ResourceStorage]
	}
	if len(rr.data.Destination.ReplicationDestinationVolumeOptions.AccessModes) > 0 {
		accessModes = rr.data.Destination.ReplicationDestinationVolumeOptions.AccessModes
	}
	if rr.data.Destination.ReplicationDestinationVolumeOptions.Capacity != nil {
		capacity = *rr.data.Destination.ReplicationDestinationVolumeOptions.Capacity
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rr.data.Destination.RDName,
			Namespace: rr.data.Destination.Namespace,
		},
	}

	// We use CoU so that we will (1) gracefully fail the create if it already
	// exists, (2) create it if it doesn't, (3) have a copy of the object either
	// way
	_, err := ctrlutil.CreateOrUpdate(ctx, c, pvc, func() error {
		// Only modify the PVC if we're creating it
		if !pvc.CreationTimestamp.IsZero() {
			return nil
		}

		klog.Infof("creating destination PVC: %v/%v", pvc.Namespace, pvc.Name)
		pvc.Spec = corev1.PersistentVolumeClaimSpec{
			AccessModes: accessModes,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: capacity,
				},
			},
			StorageClassName: rr.data.Destination.ReplicationDestinationVolumeOptions.StorageClassName,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return pvc, nil
}
