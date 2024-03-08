/*
Copyright © 2022 The VolSync authors

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
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

type replicationSetDestination struct {
	rel *replicationRelationship
	// Parsed CLI options
	accessModes             []corev1.PersistentVolumeAccessMode
	capacity                *resource.Quantity
	copyMethod              volsyncv1alpha1.CopyMethodType
	destName                XClusterName
	serviceType             corev1.ServiceType
	storageClassName        *string
	volumeSnapshotClassName *string
}

// replicationSetDestinationCmd represents the replicationSetDestination command
var replicationSetDestinationCmd = &cobra.Command{
	Use:   "set-destination",
	Short: i18n.T("Set the destination of the replication"),
	Long: templates.LongDesc(i18n.T(`
	This command sets the destination of the replication.
	`)),
	RunE: func(cmd *cobra.Command, _ []string) error {
		rsd, err := newReplicationSetDestination(cmd)
		if err != nil {
			return err
		}
		rsd.rel, err = loadReplicationRelationship(cmd)
		if err != nil {
			return err
		}
		return rsd.Run(cmd.Context())
	},
}

func init() {
	replicationCmd.AddCommand(replicationSetDestinationCmd)

	replicationSetDestinationCmd.Flags().StringSlice("accessmodes", []string{},
		"volume access modes for the destination PVC (e.g. ReadWriteOnce, ReadWriteMany)")
	replicationSetDestinationCmd.Flags().String("capacity", "",
		"capacity to allocate for the destination (e.g., \"10Gi\")")
	replicationSetDestinationCmd.Flags().String("copymethod", "Snapshot", "method used to create a point-in-time copy")
	replicationSetDestinationCmd.Flags().String("destination", "", "name of the destination: [context/]namespace/name")
	cobra.CheckErr(replicationSetDestinationCmd.MarkFlagRequired("destination"))
	replicationSetDestinationCmd.Flags().String("servicetype", "ClusterIP",
		"type of Service to create for incoming connections (ClusterIP | LoadBalancer)")
	replicationSetDestinationCmd.Flags().String("storageclass", "",
		"name of the StorageClass to use for the destination volume")
	replicationSetDestinationCmd.Flags().String("volumesnapshotclass", "",
		"name of the VolumeSnapshotClass to use for destination snapshots")
}

func newReplicationSetDestination(cmd *cobra.Command) (*replicationSetDestination, error) {
	var err error
	rsd := &replicationSetDestination{}

	if rsd.accessModes, err = parseAccessModes(cmd.Flags(), "accessmodes"); err != nil {
		return nil, err
	}

	cm, err := parseCopyMethod(cmd.Flags(), "copymethod", false)
	if err != nil {
		return nil, err
	}
	rsd.copyMethod = *cm

	rsd.capacity, err = parseCapacity(cmd.Flags(), "capacity")
	if err != nil {
		return nil, err
	}

	destname, err := cmd.Flags().GetString("destination")
	if err != nil {
		return nil, err
	}
	xcr, err := ParseXClusterName(destname)
	if err != nil {
		return nil, err
	}
	rsd.destName = *xcr

	svc, err := cmd.Flags().GetString("servicetype")
	if err != nil {
		return nil, err
	}
	if svc != string(corev1.ServiceTypeLoadBalancer) && svc != string(corev1.ServiceTypeClusterIP) {
		return nil, fmt.Errorf("servicetype must be LoadBalancer or ClusterIP")
	}
	rsd.serviceType = corev1.ServiceType(svc)

	scName, err := cmd.Flags().GetString("storageclass")
	if err != nil {
		return nil, err
	}
	if len(scName) > 0 {
		rsd.storageClassName = &scName
	}

	vscName, err := cmd.Flags().GetString("volumesnapshotclass")
	if err != nil {
		return nil, err
	}
	if len(vscName) > 0 {
		rsd.volumeSnapshotClassName = &vscName
	}

	return rsd, nil
}

func (rsd *replicationSetDestination) Run(ctx context.Context) error {
	// Since we're changing the destination, we should delete the old resources
	// (if they exist)
	srcClient, dstClient, _ := rsd.rel.GetClients()
	// Best effort to delete. Error status doesn't affect what we need to do
	_ = rsd.rel.DeleteSource(ctx, srcClient)
	_ = rsd.rel.DeleteDestination(ctx, dstClient)

	rsd.rel.data.Destination = &replicationRelationshipDestinationV2{
		Cluster:   rsd.destName.Cluster,
		Namespace: rsd.destName.Namespace,
		RDName:    rsd.destName.Name,
		ReplicationDestinationVolumeOptions: volsyncv1alpha1.ReplicationDestinationVolumeOptions{
			AccessModes:             rsd.accessModes,
			CopyMethod:              rsd.copyMethod,
			Capacity:                rsd.capacity,
			StorageClassName:        rsd.storageClassName,
			VolumeSnapshotClassName: rsd.volumeSnapshotClassName,
		},
		ServiceType: &rsd.serviceType,
	}

	var err error
	if err = rsd.rel.Save(); err != nil {
		return fmt.Errorf("unable to save relationship configuration: %w", err)
	}

	return err
}
