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
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
)

// migrationCreateCmd represents the create command
var migrationCreateCmd = &cobra.Command{
	Use:   "create",
	Short: i18n.T("Create a new migration destination"),
	Long: templates.LongDesc(i18n.T(`
	This command creates and prepares new migration destination to receive data.

	It creates the named PersistentVolumeClaim if it does not already exist,
	and it sets up an associated ReplicationDestination that will be configured
	to accept incoming transfers via rsync over ssh.
	`)),
	RunE: doMigrationCreate,
	Args: validateMigrationCreate,
}

func init() {
	migrationCmd.AddCommand(migrationCreateCmd)

	migrationCreateCmd.Flags().String("capacity", "", "capacity of the PVC to create")
	cobra.CheckErr(viper.BindPFlag("capacity", migrationCreateCmd.Flags().Lookup("capacity")))
	migrationCreateCmd.Flags().String("storageclass", "", "StorageClass name for the PVC")
	cobra.CheckErr(viper.BindPFlag("storageclass", migrationCreateCmd.Flags().Lookup("storageclass")))
}

func validateMigrationCreate(cmd *cobra.Command, args []string) error {
	// If specified, the PVC's capacity must parse to a valid resource.Quantity
	capacity := cmd.Flags().Lookup("capacity").Value.String()
	if len(capacity) > 0 {
		if _, err := resource.ParseQuantity(capacity); err != nil {
			return fmt.Errorf("capacity must be a valid resource.Quantity: %w", err)
		}
	}
	return nil
}

func doMigrationCreate(cmd *cobra.Command, args []string) error {
	// Create the empty relationship
	configDir := viper.GetString("config-dir")
	rName := viper.GetString("relationship")
	relation, err := CreateRelationship(configDir, rName, MigrationRelationship)
	if err != nil {
		return err
	}

	relation.Set("capacity", viper.GetString("capacity"))

	if err = relation.Save(); err != nil {
		return fmt.Errorf("unable to save relationship configuration: %w", err)
	}

	// TODO: Set up objects on the cluster

	return nil
}
