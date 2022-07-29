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

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/controller-runtime/pkg/client"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

type pvBackupDelete struct {
	pr     *pvBackupRelationship
	client client.Client
}

var pvBackupDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: i18n.T("Delete backup/restore relationship resources"),
	Long: templates.LongDesc(i18n.T(`
	This command removes the relatioship resources built as part of pv-back create 
	and pv-back restore command.

	ex: kubectl volsync pv-backup delete --relationship pvb1
	`)),
	RunE: func(cmd *cobra.Command, args []string) error {
		pd, err := newPVBackupDelete(cmd)
		if err != nil {
			return err
		}
		pr, err := loadPVBackupRelationship(cmd)
		if err != nil {
			return err
		}
		pd.pr = pr
		return pd.Run(cmd.Context())
	},
}

func init() {
	initPVBackupDeleteCmd(pvBackupDeleteCmd)
}

func initPVBackupDeleteCmd(pvBackupDeleteCmd *cobra.Command) {
	pvBackupCmd.AddCommand(pvBackupDeleteCmd)
}

func newPVBackupDelete(cmd *cobra.Command) (*pvBackupDelete, error) {
	return &pvBackupDelete{}, nil
}

func (pd *pvBackupDelete) Run(ctx context.Context) error {
	// Path for deletion of ReplicationSource releated objects
	prs := pd.pr.data.Source
	if prs != nil && len(prs.RSName) > 0 {
		client, err := newClient(pd.pr.data.Source.Cluster)
		if err != nil {
			return err
		}
		pd.client = client
		err = pd.deleteReplicationSource(ctx)
		if err != nil {
			return err
		}

		ns := types.NamespacedName{
			Namespace: prs.Namespace,
			Name:      prs.Source.Repository}
		// Delete the Secret
		err = deleteSecret(ctx, ns, pd.client)
		if err != nil {
			return err
		}
	}

	// Path for deletion of ReplicationDestination releated objects
	prd := pd.pr.data.Destination
	if prd != nil && len(prd.RDName) > 0 {
		client, err := newClient(pd.pr.data.Destination.Cluster)
		if err != nil {
			return err
		}
		pd.client = client
		err = pd.deleteReplicationDestination(ctx)
		if err != nil {
			return err
		}

		// Delete the Secret
		ns := types.NamespacedName{
			Namespace: prd.Namespace,
			Name:      prd.Destination.Repository}
		err = deleteSecret(ctx, ns, pd.client)
		if err != nil {
			return err
		}
	}
	// Delete the relationship file
	err := pd.pr.Delete()
	if err != nil {
		return err
	}

	return nil
}

func (pd *pvBackupDelete) deleteReplicationSource(ctx context.Context) error {
	prs := pd.pr.data.Source
	rs := &volsyncv1alpha1.ReplicationSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      prs.RSName,
			Namespace: prs.Namespace,
		},
	}
	err := pd.client.Delete(ctx, rs)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			klog.Info("backup source %s not found, ignore", prs.RSName)
			return nil
		}
		return fmt.Errorf("failed to get source CR %w", err)
	}

	klog.Infof("Deleted backup relationship resources: \"%s\"", prs.RSName)
	return nil
}

func (pd *pvBackupDelete) deleteReplicationDestination(ctx context.Context) error {
	prd := pd.pr.data.Destination
	rd := &volsyncv1alpha1.ReplicationDestination{
		ObjectMeta: metav1.ObjectMeta{
			Name:      prd.RDName,
			Namespace: prd.Namespace,
		},
	}

	err := pd.client.Delete(ctx, rd)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			klog.Info("restore destination %s not found, ignore", prd.RDName)
			return nil
		}
		return err
	}

	klog.Infof("Deleted ReplicationDestination: \"%s\"", prd.RDName)
	return nil
}
