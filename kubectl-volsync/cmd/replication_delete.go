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
	"fmt"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type replicationDelete struct {
	cobra.Command
	rel *Relationship
}

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
	RunE: func(cmd *cobra.Command, args []string) error {
		r := &replicationDelete{
			Command: *cmd,
		}
		return r.Run()
	},
}

func init() {
	replicationCmd.AddCommand(replicationDeleteCmd)
}

func (cmd *replicationDelete) Run() error {
	var err error
	cmd.rel, err = LoadRelationshipFromCommand(&cmd.Command, ReplicationRelationship)
	if err != nil {
		return err
	}

	if obj, err := XClusterNameFromRelationship(cmd.rel, "source"); err == nil {
		cl, err := newClient(obj.Cluster)
		if err != nil {
			return fmt.Errorf("unable to create client for cluster context: %s: %w", obj.Cluster, err)
		}
		rs := volsyncv1alpha1.ReplicationSource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      obj.Name,
				Namespace: obj.Namespace,
			},
		}
		err = cl.Delete(cmd.Context(), &rs, client.PropagationPolicy(metav1.DeletePropagationForeground))
		if err != nil {
			fmt.Printf("error removing ReplicationSource %v: %v\n", obj, err)
		}
	}

	if obj, err := XClusterNameFromRelationship(cmd.rel, "destination"); err == nil {
		cl, err := newClient(obj.Cluster)
		if err != nil {
			return fmt.Errorf("unable to create client for cluster context: %s: %w", obj.Cluster, err)
		}
		rs := volsyncv1alpha1.ReplicationDestination{
			ObjectMeta: metav1.ObjectMeta{
				Name:      obj.Name,
				Namespace: obj.Namespace,
			},
		}
		err = cl.Delete(cmd.Context(), &rs, client.PropagationPolicy(metav1.DeletePropagationForeground))
		if err != nil {
			fmt.Printf("error removing ReplicationDestination %v: %v\n", obj, err)
		}
	}

	if err := cmd.rel.Delete(); err != nil {
		return err
	}

	return nil
}
