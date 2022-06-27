/*
Copyright © 2022 The VolSync authors

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

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// pvBackupRelationship defines the "type" of pvBackup Relationships
const PVBackupRelationshipType RelationshipType = "PVBackup"

// pvBackupRelationship holds the config state for pvBackup-type
// relationships
type pvBackupRelationship struct {
	Relationship
	data *pvBackupRelationshipData
}

// pvBackupRelationshipData is the state that will be saved to the
// relationship config file
type pvBackupRelationshipData struct {
	Version int
	// Config info for the source side of the relationship
	Source *pvBackupRelationshipSource
	// Config info for the destination side of the relationship
	Destination *pvBackupRelationshipDestination
}

type pvBackupRelationshipSource struct {
	// Cluster context name
	Cluster string
	// Namespace on source cluster
	Namespace string
	// Name of PVC to be backed up
	PVCName string
	// Name of ReplicationSource object
	RSName string
	// Parameters for the ReplicationSource
	Source volsyncv1alpha1.ReplicationSourceResticSpec
	// Scheduling parameters
	Trigger volsyncv1alpha1.ReplicationSourceTriggerSpec
}

type pvBackupRelationshipDestination struct {
	// Cluster context name
	Cluster string
	// Namespace on destination cluster
	Namespace string
	// Name of PVC to be restored
	PVCName string
	// Name of the ReplicationDestination object
	RDName string
	// Parameters for the ReplicationDestination
	Destination volsyncv1alpha1.ReplicationDestinationResticSpec
	// Scheduling parameters
	Trigger volsyncv1alpha1.ReplicationDestinationTriggerSpec
}

func (pr *pvBackupRelationship) Save() error {
	err := pr.SetData(pr.data)
	if err != nil {
		return err
	}
	return pr.Relationship.Save()
}

func newPVBackupRelationship(cmd *cobra.Command) (*pvBackupRelationship, error) {
	r, err := CreateRelationshipFromCommand(cmd, PVBackupRelationshipType)
	if err != nil {
		return nil, err
	}

	return &pvBackupRelationship{
		Relationship: *r,
		data: &pvBackupRelationshipData{
			Version: 1,
		},
	}, nil
}

// pvBackupCmd represents the pvBackup command
var pvBackupCmd = &cobra.Command{
	Use:   "pv-backup",
	Short: i18n.T("Back up/Restore data into/from a restic repository"),
	Long: templates.LongDesc(i18n.T(`
	Automated backup/restore data from a restic repository.

	This set of commands is designed to configure the restic repository to provide
	automatic backups and restore`)),
}

func init() {
	rootCmd.AddCommand(pvBackupCmd)

	pvBackupCmd.PersistentFlags().StringP("relationship", "r", "", "relationship name")
	cobra.CheckErr(pvBackupCmd.MarkPersistentFlagRequired("relationship"))
}

func loadPVBackupRelationship(cmd *cobra.Command) (*pvBackupRelationship, error) {
	r, err := LoadRelationshipFromCommand(cmd, PVBackupRelationshipType)
	if err != nil {
		return nil, err
	}

	pr := &pvBackupRelationship{
		Relationship: *r,
	}

	// Decode according to the file version
	version := pr.GetInt("data.version")
	switch version {
	case 1:
		if err := pr.GetData(&pr.data); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported config file version %d", version)
	}
	return pr, nil
}

func (pr *pvBackupRelationship) Apply(ctx context.Context, srcClient client.Client) error {
	klog.Infof("Applying new schedule on Source")
	rs := &volsyncv1alpha1.ReplicationSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pr.data.Source.RSName,
			Namespace: pr.data.Source.Namespace,
		},
	}
	_, err := ctrlutil.CreateOrUpdate(ctx, srcClient, rs, func() error {
		pr.AddIDLabel(rs)
		rs.Spec = volsyncv1alpha1.ReplicationSourceSpec{
			SourcePVC: pr.data.Source.PVCName,
			Trigger:   &pr.data.Source.Trigger,
			Restic: &volsyncv1alpha1.ReplicationSourceResticSpec{
				Repository: pr.data.Source.Source.Repository,
				ReplicationSourceVolumeOptions: volsyncv1alpha1.ReplicationSourceVolumeOptions{
					CopyMethod: pr.data.Source.Source.CopyMethod,
				},
			},
		}

		return nil
	})
	return err
}

func (prs *pvBackupRelationshipSource) getReplicationSource(ctx context.Context, cl client.Client) (
	*volsyncv1alpha1.ReplicationSource, error) {
	nsName := types.NamespacedName{
		Namespace: prs.Namespace,
		Name:      prs.RSName,
	}
	rs := &volsyncv1alpha1.ReplicationSource{}
	err := cl.Get(ctx, nsName, rs)
	if err != nil {
		return nil, err
	}

	return rs, nil
}

func (prd *pvBackupRelationshipDestination) getReplicationDestination(ctx context.Context,
	client client.Client) (
	*volsyncv1alpha1.ReplicationDestination, error) {
	nsName := types.NamespacedName{
		Namespace: prd.Namespace,
		Name:      prd.RDName,
	}
	rd := &volsyncv1alpha1.ReplicationDestination{}
	err := client.Get(ctx, nsName, rd)
	if err != nil {
		return nil, err
	}

	return rd, nil
}
