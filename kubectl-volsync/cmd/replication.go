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
	"time"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	errorsutil "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
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
	data replicationRelationshipData
}

// replicationRelationshipData is the state that will be saved to the
// relationship config file
type replicationRelationshipData struct {
	// Config file/struct version used so we know how to decode when parsing
	// from disk
	Version int
	// Config info for the source side of the relationship
	Source *replicationRelationshipSource
	// Config info for the destination side of the relationship
	Destination *replicationRelationshipDestination
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

	replicationCmd.PersistentFlags().StringP("relationship", "r", "", "relationship name")
	cobra.CheckErr(replicationCmd.MarkPersistentFlagRequired("relationship"))
}

func newReplicationRelationship(cmd *cobra.Command) (*replicationRelationship, error) {
	r, err := CreateRelationshipFromCommand(cmd, ReplicationRelationshipType)
	if err != nil {
		return nil, err
	}

	return &replicationRelationship{
		Relationship: *r,
		data: replicationRelationshipData{
			Version: 1,
		},
	}, nil
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
		if err := rr.GetData(&rr.data); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported config file version %d", version)
	}
	return rr, nil
}

func (rr *replicationRelationship) Save() error {
	if err := rr.SetData(rr.data); err != nil {
		return err
	}
	// resource.Quantity doesn't properly encode, so we need to do it manually
	if rr.data.Source != nil && rr.data.Source.Source.Capacity != nil {
		rr.Set("data.source.source.replicationsourcevolumeoptions.capacity",
			rr.data.Source.Source.Capacity.String())
	}
	if rr.data.Destination != nil && rr.data.Destination.Destination.Capacity != nil {
		rr.Set("data.destination.destination.replicationdestinationvolumeoptions.capacity",
			rr.data.Destination.Destination.Capacity.String())
	}
	return rr.Relationship.Save()
}

// GetClients returns clients to access the src & dst clusters (srcClient,
// dstClient, error)
func (rr *replicationRelationship) GetClients() (client.Client, client.Client, error) {
	var srcClient, dstClient client.Client
	var err error
	errList := []error{}
	if rr.data.Source != nil {
		if srcClient, err = newClient(rr.data.Source.Cluster); err != nil {
			klog.Errorf("unable to create client for source cluster: %w", err)
			errList = append(errList, err)
		}
	}
	if rr.data.Destination != nil {
		if dstClient, err = newClient(rr.data.Destination.Cluster); err != nil {
			klog.Errorf("unable to create client for destination cluster: %w", err)
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
			klog.Errorf("unable to remove previous Source objects: %w", err)
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
		klog.Errorf("unable to remove previous Destination objects: %w", err)
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
	if rr.data.Destination.Destination.CopyMethod == volsyncv1alpha1.CopyMethodSnapshot {
		// We need to ensure the RD has defaults based on the source volume
		if rr.data.Destination.Destination.Capacity == nil {
			capacity := srcPVC.Spec.Resources.Requests[corev1.ResourceStorage]
			rr.data.Destination.Destination.Capacity = &capacity
		}
		if len(rr.data.Destination.Destination.AccessModes) == 0 {
			rr.data.Destination.Destination.AccessModes = srcPVC.Spec.AccessModes
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

	address, keys, err := rr.applyDestination(ctx, dstClient, dstPVC)
	if err != nil {
		return err
	}

	return rr.applySource(ctx, srcClient, address, keys)
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
	if len(rr.data.Destination.Destination.AccessModes) > 0 {
		accessModes = rr.data.Destination.Destination.AccessModes
	}
	if rr.data.Destination.Destination.Capacity != nil {
		capacity = *rr.data.Destination.Destination.Capacity
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
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: capacity,
				},
			},
			StorageClassName: rr.data.Destination.Destination.StorageClassName,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return pvc, nil
}

func (rr *replicationRelationship) applyDestination(ctx context.Context,
	c client.Client, dstPVC *corev1.PersistentVolumeClaim) (*string, *corev1.Secret, error) {
	params := rr.data.Destination

	// Create destination
	rd := &volsyncv1alpha1.ReplicationDestination{
		ObjectMeta: metav1.ObjectMeta{
			Name:      params.RDName,
			Namespace: params.Namespace,
		},
	}
	_, err := ctrlutil.CreateOrUpdate(ctx, c, rd, func() error {
		rr.AddIDLabel(rd)
		rd.Spec = volsyncv1alpha1.ReplicationDestinationSpec{
			Rsync: &params.Destination,
		}
		if dstPVC != nil {
			rd.Spec.Rsync.DestinationPVC = &dstPVC.Name
		}
		return nil
	})
	if err != nil {
		klog.Errorf("unable to create ReplicationDestination: %w", err)
		return nil, nil, err
	}

	rd, err = rr.awaitDestAddrKeys(ctx, c, client.ObjectKeyFromObject(rd))
	if err != nil {
		klog.Errorf("error while waiting for destination keys and address: %w", err)
		return nil, nil, err
	}

	// Fetch the keys
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      *rd.Status.Rsync.SSHKeys,
			Namespace: params.Namespace,
		},
	}
	if err = c.Get(ctx, client.ObjectKeyFromObject(secret), secret); err != nil {
		klog.Errorf("unable to retrieve ssh keys: %w", err)
		return nil, nil, err
	}

	return rd.Status.Rsync.Address, secret, nil
}

func (rr *replicationRelationship) awaitDestAddrKeys(ctx context.Context, c client.Client,
	rdName types.NamespacedName) (*volsyncv1alpha1.ReplicationDestination, error) {
	klog.Infof("waiting for keys & address of destination to be available")
	rd := volsyncv1alpha1.ReplicationDestination{}
	err := wait.PollImmediate(5*time.Second, 5*time.Minute, func() (bool, error) {
		if err := c.Get(ctx, rdName, &rd); err != nil {
			return false, err
		}
		if rd.Status == nil || rd.Status.Rsync == nil {
			return false, nil
		}
		if rd.Status.Rsync.Address == nil {
			return false, nil
		}
		if rd.Status.Rsync.SSHKeys == nil {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return &rd, nil
}

func (rr *replicationRelationship) applySource(ctx context.Context, c client.Client,
	address *string, dstKeys *corev1.Secret) error {
	klog.Infof("creating resources on Source")
	srcKeys, err := rr.applySourceKeys(ctx, c, dstKeys)
	if err != nil {
		klog.Errorf("unable to create source ssh keys: %w", err)
		return err
	}

	rs := &volsyncv1alpha1.ReplicationSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rr.data.Source.RSName,
			Namespace: rr.data.Source.Namespace,
		},
	}
	_, err = ctrlutil.CreateOrUpdate(ctx, c, rs, func() error {
		rr.AddIDLabel(rs)
		rs.Spec = volsyncv1alpha1.ReplicationSourceSpec{
			SourcePVC: rr.data.Source.PVCName,
			Trigger:   &rr.data.Source.Trigger,
			Rsync:     &rr.data.Source.Source,
		}
		rs.Spec.Rsync.Address = address
		rs.Spec.Rsync.SSHKeys = &srcKeys.Name
		return nil
	})
	return err
}

// Copies the ssh keys into the source cluster
func (rr *replicationRelationship) applySourceKeys(ctx context.Context,
	c client.Client, dstKeys *corev1.Secret) (*corev1.Secret, error) {
	srcKeys := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rr.data.Source.RSName,
			Namespace: rr.data.Source.Namespace,
		},
	}
	_, err := ctrlutil.CreateOrUpdate(ctx, c, srcKeys, func() error {
		rr.AddIDLabel(srcKeys)
		srcKeys.Data = dstKeys.Data
		return nil
	})
	if err != nil {
		return nil, err
	}
	return srcKeys, nil
}
