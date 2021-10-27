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

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/spf13/cobra"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type replicationDelete struct {
	rel *replicationRelationship
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
		r, err := newReplicationDelete(cmd)
		if err != nil {
			return err
		}
		r.rel, err = loadReplicationRelationship(cmd)
		if err != nil {
			return err
		}
		return r.Run(cmd.Context())
	},
}

func init() {
	replicationCmd.AddCommand(replicationDeleteCmd)
}

func newReplicationDelete(cmd *cobra.Command) (*replicationDelete, error) {
	rdel := &replicationDelete{}
	return rdel, nil
}

func (rdel *replicationDelete) Run(ctx context.Context) error {
	if rdel.rel.data.Source != nil {
		cl, err := newClient(rdel.rel.data.Source.Cluster)
		if err != nil {
			return fmt.Errorf("unable to create client for source cluster context: %w", err)
		}
		rs := volsyncv1alpha1.ReplicationSource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      rdel.rel.data.Source.RSName,
				Namespace: rdel.rel.data.Source.Namespace,
			},
		}
		klog.Infof("deleting ReplicationSource: %v", client.ObjectKeyFromObject(&rs))
		err = cl.Delete(ctx, &rs, client.PropagationPolicy(metav1.DeletePropagationForeground))
		if err != nil && !kerrors.IsNotFound(err) {
			fmt.Printf("error removing ReplicationSource %v: %v\n", rs, err)
		}
	}

	if rdel.rel.data.Destination != nil {
		cl, err := newClient(rdel.rel.data.Destination.Cluster)
		if err != nil {
			return fmt.Errorf("unable to create client for destination cluster context: %w", err)
		}
		rd := volsyncv1alpha1.ReplicationDestination{
			ObjectMeta: metav1.ObjectMeta{
				Name:      rdel.rel.data.Destination.RDName,
				Namespace: rdel.rel.data.Destination.Namespace,
			},
		}
		klog.Infof("deleting ReplicationDestination: %v", client.ObjectKeyFromObject(&rd))
		err = cl.Delete(ctx, &rd, client.PropagationPolicy(metav1.DeletePropagationForeground))
		if err != nil && !kerrors.IsNotFound(err) {
			fmt.Printf("error removing ReplicationDestination %v: %v\n", rd, err)
		}
	}

	if err := rdel.rel.Delete(); err != nil {
		return err
	}

	return nil
}
