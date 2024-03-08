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

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/util/i18n"
	"sigs.k8s.io/controller-runtime/pkg/client"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

type migrationDelete struct {
	// migration relationship object to be persisted to a config file
	mr *migrationRelationship
	// client object to communicate with a cluster
	client client.Client
}

// migrationDeleteCmd represents the delete command
var migrationDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: i18n.T("Delete a new migration destination"),
	Long: `This command deletes the Replication destination
	and the relationship file`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		md := newMigrationDelete()
		mr, err := loadMigrationRelationship(cmd)
		if err != nil {
			return err
		}
		md.mr = mr
		return md.Run(cmd.Context())
	},
}

func init() {
	migrationCmd.AddCommand(migrationDeleteCmd)
}

func (md *migrationDelete) Run(ctx context.Context) error {
	client, err := newClient(md.mr.data.Destination.Cluster)
	if err != nil {
		return err
	}
	md.client = client

	// Delete the ReplicationDestination
	err = md.deleteReplicationDestination(ctx)
	if err != nil {
		return err
	}

	// Delete the relationship file
	err = md.mr.Delete()
	if err != nil {
		return err
	}

	return nil
}

func newMigrationDelete() *migrationDelete {
	return &migrationDelete{}
}

func (md *migrationDelete) deleteReplicationDestination(ctx context.Context) error {
	mrd := md.mr.data.Destination

	rd := &volsyncv1alpha1.ReplicationDestination{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mrd.RDName,
			Namespace: mrd.Namespace,
		},
	}

	err := md.client.Delete(ctx, rd)
	if err != nil {
		return err // Note this will return error if the RD doesn't exist
	}

	klog.Infof("Deleted ReplicationDestination: \"%s\"", mrd.RDName)
	return nil
}
