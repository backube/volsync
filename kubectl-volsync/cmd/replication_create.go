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

	"github.com/spf13/cobra"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
)

// replicationCreateCmd represents the create command
var replicationCreateCmd = &cobra.Command{
	Use:   "create",
	Short: i18n.T("Create a new replication relationship"),
	Long: templates.LongDesc(i18n.T(`
	This command creates a new, empty replication relationship.

	Once created, both a source (set-source) and a destination (set-destination)
	must be added.
	`)),
	RunE: doReplicationCreate,
}

func init() {
	replicationCmd.AddCommand(replicationCreateCmd)
}

func doReplicationCreate(cmd *cobra.Command, args []string) error {
	configDir, err := cmd.Flags().GetString("config-dir")
	if err != nil {
		return err
	}
	rName, err := cmd.Flags().GetString("relationship")
	if err != nil {
		return err
	}
	relation, err := CreateRelationship(configDir, rName, ReplicationRelationship)
	if err != nil {
		return err
	}
	if err = relation.Save(); err != nil {
		return fmt.Errorf("unable to save relationship configuration: %w", err)
	}

	return nil
}
