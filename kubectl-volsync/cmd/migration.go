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
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/component-base/logs"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/controller-runtime/pkg/client"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

// MigrationRelationship defines the "type" of migration Relationships
const MigrationRelationshipType RelationshipType = "migration"

const (
	defaultLocalStunnelPort       = 9000
	defaultDestinationStunnelPort = 8000
)

// migrationRelationship holds the config state for migration-type
// relationships
type migrationRelationship struct {
	Relationship
	data *migrationRelationshipDataV2
	mh   migrationHandler
}

type migrationHandler interface {
	EnsureReplicationDestination(ctx context.Context, c client.Client,
		destConfig *migrationRelationshipDestinationV2) (*volsyncv1alpha1.ReplicationDestination, error)
	WaitForRDStatus(ctx context.Context, c client.Client,
		replicationDestination *volsyncv1alpha1.ReplicationDestination) (*volsyncv1alpha1.ReplicationDestination, error)
	RunMigration(ctx context.Context, c client.Client, source string, destConfig *migrationRelationshipDestinationV2) error
}

// Old v1 version of the data
type migrationRelationshipData struct {
	Version     int
	Destination *migrationRelationshipDestination
}

// migrationRelationshipData is the state that will be saved to the
// relationship config file
type migrationRelationshipDataV2 struct {
	Version int
	// True if the ReplicationDestination should use RsyncTLS
	IsRsyncTLS  bool
	Destination *migrationRelationshipDestinationV2
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

type migrationRelationshipDestinationV2 struct {
	// Cluster context name
	Cluster string
	// Namespace on destination cluster
	Namespace string
	// Name of PVC being replicated
	PVCName string
	// Name of the ReplicationDestination object
	RDName string
	// Name of Secret holding ssh or psk secret
	//RsyncSecretName string //TODO: is this necessary? doesn't seem to get written to conf file in ~/.volsync
	// Service Type for the ReplicationDestination
	ServiceType *corev1.ServiceType
	// Copy Method for the ReplicationDestination (will always be Direct for migration)
	CopyMethod volsyncv1alpha1.CopyMethodType
	// MoverSecurityContext allows specifying the PodSecurityContext that will
	// be used by the data mover
	MoverSecurityContext *corev1.PodSecurityContext
}

func (mr *migrationRelationship) Save() error {
	err := mr.SetData(mr.data)
	if err != nil {
		return err
	}

	return mr.Relationship.Save()
}

func (mr *migrationRelationship) convertDataToV2(datav1 *migrationRelationshipData) {
	mr.data = &migrationRelationshipDataV2{
		Version:    2,
		IsRsyncTLS: false, // Rsync TLS support wasn't there in v1
		Destination: &migrationRelationshipDestinationV2{
			RDName:      datav1.Destination.RDName,
			PVCName:     datav1.Destination.PVCName,
			Namespace:   datav1.Destination.Namespace,
			Cluster:     datav1.Destination.Cluster,
			ServiceType: datav1.Destination.Destination.ServiceType,
			CopyMethod:  volsyncv1alpha1.CopyMethodDirect, // Default, but wasn't specified in v1
		},
	}
}

func newMigrationRelationship(cmd *cobra.Command) (*migrationRelationship, error) {
	r, err := CreateRelationshipFromCommand(cmd, MigrationRelationshipType)
	if err != nil {
		return nil, err
	}

	return &migrationRelationship{
		Relationship: *r,
		data: &migrationRelationshipDataV2{
			Version: 2,
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

	// Add logging flags to all sub-commands
	logs.AddFlags(migrationCmd.PersistentFlags())

	migrationCmd.PersistentFlags().StringP("relationship", "r", "", "relationship name")
	cobra.CheckErr(migrationCmd.MarkPersistentFlagRequired("relationship"))
	cobra.CheckErr(viper.BindPFlag("relationship", migrationCmd.PersistentFlags().Lookup("relationship")))
}

func loadMigrationRelationship(cmd *cobra.Command) (*migrationRelationship, error) {
	r, err := LoadRelationshipFromCommand(cmd, MigrationRelationshipType)
	if err != nil {
		return nil, err
	}

	mr := &migrationRelationship{
		Relationship: *r,
	}

	// Decode according to the file version
	version := mr.GetInt("data.version")
	switch version {
	case 1:
		// version2 is now the default, read in the v1 data and migrate to v2
		datav1 := &migrationRelationshipData{}
		if err := mr.GetData(&datav1); err != nil {
			return nil, err
		}
		mr.convertDataToV2(datav1) // Convert from v1 to v2
	case 2:
		if err := mr.GetData(&mr.data); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported config file version %d", version)
	}

	if mr.data.IsRsyncTLS {
		mr.mh = &migrationHandlerRsyncTLS{}
	} else {
		mr.mh = &migrationHandlerRsync{}
	}

	return mr, nil
}
