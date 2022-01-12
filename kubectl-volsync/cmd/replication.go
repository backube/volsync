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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	errorsutil "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
