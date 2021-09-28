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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/controller-runtime/pkg/client"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

// replicationDeleteCmd represents the delete command
var replicationDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: i18n.T("Delete an existing replication relationship"),
	Long: templates.LongDesc(i18n.T(`
	This command deletes a replication relationship and removes the associated
	objects from the cluster(s).

	The delete command removes the VolSync CRs for this relationship as well as
	any associated VolumeSnapshot objects. In order to preserve the replicated
	data, the destination should be "promote"-ed prior to deleting the
	relationship.
	`)),
	RunE: doReplicationDelete,
}

func init() {
	replicationCmd.AddCommand(replicationDeleteCmd)
}

func doReplicationDelete(cmd *cobra.Command, args []string) error {
	configDir, err := cmd.Flags().GetString("config-dir")
	if err != nil {
		return err
	}
	rName, err := cmd.Flags().GetString("relationship")
	if err != nil {
		return err
	}
	relation, err := LoadRelationship(configDir, rName, ReplicationRelationship)
	if err != nil {
		return err
	}

	var obj XClusterName
	if relation.IsSet("source") {
		if err := relation.UnmarshalKey("source", &obj); err == nil {
			if err := deleteReplicationSource(cmd.Context(), obj); err != nil {
				fmt.Printf("error removing ReplicationSource %v: %v\n", obj, err)
			}
		}
	}
	if relation.IsSet("destination") {
		if err := relation.UnmarshalKey("destination", &obj); err == nil {
			if err := deleteReplicationDestination(cmd.Context(), obj); err != nil {
				fmt.Printf("error removing ReplicationDestination %v: %v\n", obj, err)
			}
		}
	}

	if err := relation.Delete(); err != nil {
		return err
	}

	return nil
}

func deleteReplicationSource(ctx context.Context, rs XClusterName) error {
	cl, err := newClient(rs.Cluster)
	if err != nil {
		return err
	}
	obj := volsyncv1alpha1.ReplicationSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rs.Name,
			Namespace: rs.Namespace,
		},
	}
	err = cl.Delete(ctx, &obj, client.PropagationPolicy(metav1.DeletePropagationBackground))
	return err
}

func deleteReplicationDestination(ctx context.Context, rd XClusterName) error {
	cl, err := newClient(rd.Cluster)
	if err != nil {
		return err
	}
	obj := volsyncv1alpha1.ReplicationDestination{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rd.Name,
			Namespace: rd.Namespace,
		},
	}
	err = cl.Delete(ctx, &obj, client.PropagationPolicy(metav1.DeletePropagationBackground))
	return err
}
