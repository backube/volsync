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
	krand "k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

type replicationSetSource struct {
	rel *replicationRelationship
	// Parsed CLI options
	accessModes             []corev1.PersistentVolumeAccessMode
	copyMethod              volsyncv1alpha1.CopyMethodType
	pvcName                 XClusterName
	storageClassName        *string
	volumeSnapshotClassName *string
}

// replicationSetSourceCmd represents the replicationSetSource command
var replicationSetSourceCmd = &cobra.Command{
	Use:   "set-source",
	Short: i18n.T("Set the source of the replication"),
	Long: templates.LongDesc(i18n.T(`
	This command sets the source of the replication.
	`)),
	RunE: func(cmd *cobra.Command, args []string) error {
		rss, err := newReplicationSetSource(cmd)
		if err != nil {
			return err
		}
		rss.rel, err = loadReplicationRelationship(cmd)
		if err != nil {
			return err
		}
		return rss.Run(cmd.Context())
	},
}

func init() {
	replicationCmd.AddCommand(replicationSetSourceCmd)

	replicationSetSourceCmd.Flags().StringSlice("accessmodes", []string{},
		"volume access modes for the cloned PVC (e.g. ReadWriteOnce, ReadWriteMany)")
	replicationSetSourceCmd.Flags().String("copymethod", "Clone", "method used to create a point-in-time copy")
	replicationSetSourceCmd.Flags().String("pvcname", "", "name of the PVC to replicate: [context/]namespace/name")
	cobra.CheckErr(replicationSetSourceCmd.MarkFlagRequired("pvcname"))
	replicationSetSourceCmd.Flags().String("storageclass", "",
		"name of the StorageClass to use for the cloned volume")
	replicationSetSourceCmd.Flags().String("volumesnapshotclass", "",
		"name of the VolumeSnapshotClass to use for volume snapshots")
}

func newReplicationSetSource(cmd *cobra.Command) (*replicationSetSource, error) {
	var err error
	rss := &replicationSetSource{}

	if rss.accessModes, err = parseAccessModes(cmd.Flags(), "accessmodes"); err != nil {
		return nil, err
	}

	cm, err := parseCopyMethod(cmd.Flags(), "copymethod", true)
	if err != nil {
		return nil, err
	}
	rss.copyMethod = *cm

	pvcname, err := cmd.Flags().GetString("pvcname")
	if err != nil {
		return nil, err
	}
	xcr, err := ParseXClusterName(pvcname)
	if err != nil {
		return nil, err
	}
	rss.pvcName = *xcr

	scName, err := cmd.Flags().GetString("storageclass")
	if err != nil {
		return nil, err
	}
	if len(scName) > 0 {
		rss.storageClassName = &scName
	}

	vscName, err := cmd.Flags().GetString("volumesnapshotclass")
	if err != nil {
		return nil, err
	}
	if len(vscName) > 0 {
		rss.volumeSnapshotClassName = &vscName
	}

	return rss, nil
}

func (rss *replicationSetSource) Run(ctx context.Context) error {
	// Since we're changing the source, we should delete the old resources
	// (if they exist)
	srcClient, dstClient, _ := rss.rel.GetClients()
	// Best effort to delete. Error status doesn't affect what we need to do
	_ = rss.rel.DeleteSource(ctx, srcClient)
	_ = rss.rel.DeleteDestination(ctx, dstClient)

	rss.rel.data.Source = &replicationRelationshipSource{
		Cluster:   rss.pvcName.Cluster,
		Namespace: rss.pvcName.Namespace,
		// The RS name needs to be unique since it's possible to have a single
		// PVC be the source of multiple replications
		RSName:  rss.pvcName.Name + "-" + krand.String(5),
		PVCName: rss.pvcName.Name,
		Source: volsyncv1alpha1.ReplicationSourceRsyncSpec{
			ReplicationSourceVolumeOptions: volsyncv1alpha1.ReplicationSourceVolumeOptions{
				AccessModes:             rss.accessModes,
				CopyMethod:              rss.copyMethod,
				StorageClassName:        rss.storageClassName,
				VolumeSnapshotClassName: rss.volumeSnapshotClassName,
			},
		},
	}

	var err error
	if err = rss.rel.Save(); err != nil {
		return fmt.Errorf("unable to save relationship configuration: %w", err)
	}

	return err
}
