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
	"github.com/spf13/viper"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

// MigrationRelationship defines the "type" of migration Relationships
const MigrationRelationshipType RelationshipType = "migration"

// migrationRelationship holds the config state for migration-type
// relationships
type migrationRelationship struct {
	Relationship
	data *migrationRelationshipData
}

// migrationRelationshipData is the state that will be saved to the
// relationship config file
type migrationRelationshipData struct {
	Version     int
	Destination *migrationRelationshipDestination
}

type migrationRelationshipDestination struct {
	// Cluster context name
	Cluster string
	// Namespace on destination cluster
	Namespace string
	// Name of PVC being replicated
	PVCName string
	// Name of the ReplicationDestination object
	RDName string
	// Name of Secret holding SSH keys
	SSHKeyName string
	// Parameters for the ReplicationDestination
	Destination volsyncv1alpha1.ReplicationDestinationRsyncSpec
}

func (mr *migrationRelationship) Save() error {
	err := mr.SetData(mr.data)
	if err != nil {
		return err
	}

	if mr.data.Destination != nil && mr.data.Destination.Destination.Capacity != nil {
		mr.Set("data.destination.destination.replicationdestinationvolumeoptions.capacity",
			mr.data.Destination.Destination.Capacity.String())
	}

	return mr.Relationship.Save()
}

func newMigrationRelationship(cmd *cobra.Command) (*migrationRelationship, error) {
	r, err := CreateRelationshipFromCommand(cmd, MigrationRelationshipType)
	if err != nil {
		return nil, err
	}

	return &migrationRelationship{
		Relationship: *r,
		data: &migrationRelationshipData{
			Version: 1,
		},
	}, nil
}

// migrationCmd represents the migration command
var migrationCmd = &cobra.Command{
	Use:   "migration",
	Short: i18n.T("Migrate data into a PersistentVolume"),
	Long: templates.LongDesc(i18n.T(`
	Copy data from an external file system into a Kubernetes PersistentVolume.

	This set of commands is designed to help provision a PV and copy data from
	a directory tree into that newly provisioned volume.
	`)),
}

func init() {
	rootCmd.AddCommand(migrationCmd)

	migrationCmd.PersistentFlags().StringP("relationship", "r", "", "relationship name")
	cobra.CheckErr(migrationCmd.MarkPersistentFlagRequired("relationship"))
	cobra.CheckErr(viper.BindPFlag("relationship", migrationCmd.PersistentFlags().Lookup("relationship")))
}
