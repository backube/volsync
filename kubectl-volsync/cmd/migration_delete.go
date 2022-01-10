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
	"errors"
	"time"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/util/i18n"
)

// migrationDeleteCmd represents the delete command
var migrationDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: i18n.T("Delete a new migration destination"),
	Long: `This command delete destination relationship.

	It delete the relastionship configuration file`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return Run(cmd)
	},
}

func init() {
	migrationCmd.AddCommand(migrationDeleteCmd)
}

func Run(cmd *cobra.Command) error {
	relationship, err := LoadRelationshipFromCommand(cmd, MigrationRelationship)
	if err != nil {
		return err
	}

	err = deleteRelationshipDestination(cmd, relationship)
	if err != nil {
		return err
	}
	err = relationship.Delete()
	if err != nil {
		return err
	}
	return nil
}

func deleteRelationshipDestination(cmd *cobra.Command, relationship *Relationship) error {
	mdName := relationship.Viper.GetString("data.destination.MDName")
	cluster := relationship.Viper.GetString("data.destination.Cluster")
	namespace := relationship.Viper.GetString("data.destination.Namespace")
	klog.Infof("Relationship MDName : \"%s\" Namespace : \"%s\" and Cluster : \"%s\"",
		namespace, mdName, cluster)
	if mdName == "" || namespace == "" {
		return errors.New("Failed to get Namespace or MDName from relationship")
	}
	clinet, err := newClient(cluster)
	if err != nil {
		return err
	}
	mc := &migrationCreate{
		clientObject: clinet,
		mr: &migrationRelationship{
			data: &migrationRelationshipData{
				Destination: &migrationRelationshipDestination{
					MDName:    mdName,
					Cluster:   cluster,
					Namespace: namespace,
				},
			},
		},
	}
	err = mc.deleteReplicationDestination(cmd.Context())
	if err != nil {
		return err
	}
	return nil
}

func (mc *migrationCreate) deleteReplicationDestination(ctx context.Context) error {
	rd := mc.getDestination(ctx)
	if rd == nil {
		return errors.New("migration destination not found")
	}
	if err := mc.clientObject.Delete(ctx, rd); err != nil {
		return err
	}

	err := wait.PollImmediate(5*time.Second, 2*time.Minute, func() (bool, error) {
		rd = mc.getDestination(ctx)
		if rd != nil {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return err
	}
	klog.Infof("Deleted destination: \"%s\"", mc.mr.data.Destination.MDName)
	return nil
}

func (mc *migrationCreate) getDestination(ctx context.Context) *volsyncv1alpha1.ReplicationDestination {
	nsName := types.NamespacedName{
		Namespace: mc.mr.data.Destination.Namespace,
		Name:      mc.mr.data.Destination.MDName,
	}
	rd := &volsyncv1alpha1.ReplicationDestination{}
	err := mc.clientObject.Get(ctx, nsName, rd)
	if err == nil {
		return rd
	}

	return nil
}
