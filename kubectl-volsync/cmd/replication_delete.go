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
	errorsutil "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
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
	srcClient, dstClient, _ := rdel.rel.GetClients()
	errList := []error{}
	if err := rdel.rel.DeleteSource(ctx, srcClient); err != nil {
		errList = append(errList, err)
	}
	if err := rdel.rel.DeleteDestination(ctx, dstClient); err != nil {
		errList = append(errList, err)
	}
	if err := rdel.rel.Delete(); err != nil {
		errList = append(errList, err)
	}
	return errorsutil.NewAggregate(errList)
}
