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

const ReplicationRelationship RelationshipType = "replication"

// replicationCmd represents the replication command
var replicationCmd = &cobra.Command{
	Use:   "replication",
	Short: i18n.T("Replicate data between two PersistentVolumes"),
	Long: templates.LongDesc(i18n.T(`
	Replicate the contents of one PersistentVolume to another.

	This set of commands is designed to set up and manage a replication
	relationship between two different PVCs in the same Namespace, across
	Namespaces, or in different clusters. The contents of the volume can be
	replicated either on-demand or based on a provided schedule.
	`)),
}

func init() {
	rootCmd.AddCommand(replicationCmd)

	replicationCmd.PersistentFlags().StringP("relationship", "r", "", "relationship name")
	cobra.CheckErr(replicationCmd.MarkPersistentFlagRequired("relationship"))
}
