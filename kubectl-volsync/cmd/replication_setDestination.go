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
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/controller-runtime/pkg/client"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

type replicationSetDestination struct {
	rel *replicationRelationship
	// Parsed CLI options
	copyMethod volsyncv1alpha1.CopyMethodType
	destName   XClusterName
}

// replicationSetDestinationCmd represents the replicationSetDestination command
var replicationSetDestinationCmd = &cobra.Command{
	Use:   "set-destination",
	Short: i18n.T("Set the destination of the replication"),
	Long: templates.LongDesc(i18n.T(`
	This command sets the destination of the replication.
	`)),
	RunE: func(cmd *cobra.Command, args []string) error {
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

	replicationSetDestinationCmd.Flags().String("copymethod", "Snapshot", "method used to create a point-in-time copy")
	replicationSetDestinationCmd.Flags().String("destination", "", "name of the destination: [context/]namespace/name")
}

func newReplicationSetDestination(cmd *cobra.Command) (*replicationSetDestination, error) {
	rsd := &replicationSetDestination{}
	cm, err := cmd.Flags().GetString("copymethod")
	if err != nil {
		return nil, err
	}
	rsd.copyMethod = volsyncv1alpha1.CopyMethodType(cm)
	if rsd.copyMethod != volsyncv1alpha1.CopyMethodNone &&
		rsd.copyMethod != volsyncv1alpha1.CopyMethodSnapshot {
		return nil, fmt.Errorf("unsupported copymethod: %v", rsd.copyMethod)
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

	return rsd, nil
}

func (rsd *replicationSetDestination) Run(ctx context.Context) error {
	// Delete any previously defined Destination
	if rsd.rel.data.Destination != nil {
		dstClient, err := newClient(rsd.rel.data.Destination.Cluster)
		if err != nil {
			fmt.Printf("unable to create client for cluster context %s: %v", rsd.rel.data.Destination.Cluster, err)
		}
		if err = rsd.deleteDestination(ctx, dstClient); err != nil {
			if !kerrors.IsNotFound(err) {
				// We're unable to clean up the old, but maybe that's ok.
				fmt.Printf("unable to remove old ReplicationDestination: %v", err)
			}
		}
	}

	rsd.rel.data.Destination = &replicationRelationshipDestination{
		Cluster:   rsd.destName.Cluster,
		Namespace: rsd.destName.Namespace,
		RDName:    rsd.destName.Name,
		Destination: volsyncv1alpha1.ReplicationDestinationRsyncSpec{
			ReplicationDestinationVolumeOptions: volsyncv1alpha1.ReplicationDestinationVolumeOptions{
				CopyMethod: rsd.copyMethod,
			},
		},
	}

	if err := rsd.rel.Save(); err != nil {
		return fmt.Errorf("unable to save relationship configuration: %w", err)
	}

	return nil
}

func (rsd *replicationSetDestination) deleteDestination(ctx context.Context, c client.Client) error {
	if rsd.rel.data.Destination == nil || len(rsd.rel.data.Destination.RDName) == 0 {
		return nil
	}

	rs := volsyncv1alpha1.ReplicationDestination{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rsd.rel.data.Destination.RDName,
			Namespace: rsd.rel.data.Destination.Namespace,
		},
	}

	err := c.Delete(ctx, &rs, client.PropagationPolicy(metav1.DeletePropagationForeground))
	if err == nil || kerrors.IsNotFound(err) {
		rsd.rel.data.Destination.RDName = ""
	}
	return err
}
