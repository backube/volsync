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
	"github.com/spf13/cobra"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
)

type replicationSetSource struct {
	cobra.Command
	rel *Relationship
}

// replicationSetSourceCmd represents the replicationSetSource command
var replicationSetSourceCmd = &cobra.Command{
	Use:   "set-source",
	Short: i18n.T("Set the source of the replication"),
	Long: templates.LongDesc(i18n.T(`
	This command sets the source of the replication.
	`)),
	RunE: func(cmd *cobra.Command, args []string) error {
		r := &replicationSetSource{
			Command: *cmd,
		}
		return r.Run()
	},
}

func init() {
	replicationCmd.AddCommand(replicationSetSourceCmd)

	replicationSetSourceCmd.Flags().String("copymethod", "Clone", "method used to create a point-in-time copy")
	replicationSetSourceCmd.Flags().String("pvcname", "", "name of the PVC to replicate: [context/]namespace/name")
	cobra.CheckErr(replicationSetSourceCmd.MarkFlagRequired("pvcname"))
}

func (cmd *replicationSetSource) Run() error {
	var err error
	cmd.rel, err = LoadRelationshipFromCommand(&cmd.Command, ReplicationRelationship)
	if err != nil {
		return err
	}

	// Validate that the PVC exists
	// if a source is already defined, delete it
	// if key secret exists, delete it
	// save relationship
	// if a destination is not defined, stop
	// if destination is defined, fetch keys & address
	// create key secret
	// create source

	return nil
}
