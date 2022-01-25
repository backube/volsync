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
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
		if err := mr.GetData(&mr.data); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported config file version %d", version)
	}
	return mr, nil
}

func (mrd *migrationRelationshipDestination) waitForRDStatus(ctx context.Context, client client.Client) (
	*volsyncv1alpha1.ReplicationDestination, error) {
	// wait for migrationdestination to become ready
	var (
		rd  *volsyncv1alpha1.ReplicationDestination
		err error
	)
	err = wait.PollImmediate(5*time.Second, 2*time.Minute, func() (bool, error) {
		rd, err = mrd.getDestination(ctx, client)
		if err != nil {
			return false, err
		}
		if rd.Status == nil || rd.Status.Rsync == nil {
			return false, nil
		}
		if rd.Status.Rsync.Address == nil {
			klog.V(2).Infof("Waiting for MigrationDestination %s RSync address to populate", rd.Name)
			return false, nil
		}

		if rd.Status.Rsync.SSHKeys == nil {
			klog.V(2).Infof("Waiting for MigrationDestination %s RSync sshkeys to populate", rd.Name)
			return false, nil
		}

		klog.V(2).Infof("Found MigrationDestination RSync Address: %s", *rd.Status.Rsync.Address)
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch rd status: %w,", err)
	}

	return rd, nil
}

func (mrd *migrationRelationshipDestination) getDestination(ctx context.Context, client client.Client) (
	*volsyncv1alpha1.ReplicationDestination, error) {
	nsName := types.NamespacedName{
		Namespace: mrd.Namespace,
		Name:      mrd.RDName,
	}
	rd := &volsyncv1alpha1.ReplicationDestination{}
	err := client.Get(ctx, nsName, rd)
	if err != nil {
		return nil, err
	}

	return rd, nil
}
