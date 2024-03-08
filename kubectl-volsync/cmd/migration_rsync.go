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
	"os"

	"github.com/spf13/cobra"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type migrationSync struct {
	mr *migrationRelationship
	// Source volume to be migrated
	Source string
	// client object to communicate with a cluster
	client client.Client
}

// migrationCreateCmd represents the create command
var migrationSyncCmd = &cobra.Command{
	Use:   "rsync",
	Short: i18n.T("Rsync data from source to destination"),
	Long: templates.LongDesc(i18n.T(`
	This command ensures the migration of data from source to destination
	via rsync over ssh. The execution of this command should be followed by
	migration create which establishes the relationship.
	`)),
	RunE: func(cmd *cobra.Command, _ []string) error {
		ms, err := newMigrationSync(cmd)
		if err != nil {
			return err
		}
		mr, err := loadMigrationRelationship(cmd)
		if err != nil {
			return err
		}
		ms.mr = mr

		return ms.Run(cmd.Context())
	},
}

func init() {
	initmigrationSyncCmd(migrationSyncCmd)
}

func initmigrationSyncCmd(migrationCreateCmd *cobra.Command) {
	migrationCmd.AddCommand(migrationCreateCmd)

	migrationCreateCmd.Flags().String("source", "", "source volume to be migrated")
	cobra.CheckErr(migrationCreateCmd.MarkFlagRequired("source"))
}

func (ms *migrationSync) Run(ctx context.Context) error {
	k8sClient, err := newClient(ms.mr.data.Destination.Cluster)
	if err != nil {
		return err
	}
	ms.client = k8sClient

	// Ensure source volume
	_, err = os.Stat(ms.Source)
	if err != nil {
		return fmt.Errorf("failed to access the source volume, %w", err)
	}

	return ms.mr.mh.RunMigration(ctx, ms.client, ms.Source, ms.mr.data.Destination)
}

func newMigrationSync(cmd *cobra.Command) (*migrationSync, error) {
	ms := &migrationSync{}
	source, err := cmd.Flags().GetString("source")
	if err != nil || source == "" {
		return nil, fmt.Errorf("failed to fetch the source arg, err = %w", err)
	}
	ms.Source = source

	return ms, nil
}
