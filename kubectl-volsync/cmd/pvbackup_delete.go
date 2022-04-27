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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type pvBackupDelete struct {
	pr     *pvBackupRelationship
	client client.Client
}

var pvBackupDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: i18n.T("Delete backup relationship resources"),
	Long: templates.LongDesc(i18n.T(`
	This command removes the relatioship resources built as part of create command.

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
	client, err := newClient(pd.pr.data.Source.Cluster)
	if err != nil {
		return err
	}
	pd.client = client

	// Delete the ReplicationSource
	err = pd.deleteReplicationSource(ctx)
	if err != nil {
		return err
	}

	// Delete the Secret
	err = pd.deleteSecret(ctx)
	if err != nil {
		return err
	}
	// Delete the relationship file
	err = pd.pr.Delete()
	if err != nil {
		return err
	}

	return nil
}

func (pd *pvBackupDelete) deleteReplicationSource(ctx context.Context) error {
	prd := pd.pr.data
	rs, err := prd.getReplicationSource(ctx, pd.client)
	if err != nil {
		return fmt.Errorf("failed to get source CR returned:%w", err)
	}

	err = pd.client.Delete(ctx, rs)
	if err != nil {
		return err
	}

	klog.Infof("Deleted backup relationship resources: \"%s\"", prd.Source.RSName)
	return nil
}

func (pd *pvBackupDelete) deleteSecret(ctx context.Context) error {
	secret := &corev1.Secret{}
	ns := types.NamespacedName{
		Namespace: pd.pr.data.Source.Namespace,
		Name:      pd.pr.data.Source.Source.Repository,
	}

	err := pd.client.Get(ctx, ns, secret)
	if err != nil {
		return fmt.Errorf("secret %s not found, %w", pd.pr.data.Source.Source.Repository, err)
	}

	err = pd.client.Delete(ctx, secret)
	if err != nil {
		return fmt.Errorf("failed to delete secret %s, %w",
			pd.pr.data.Source.Source.Repository, err)
	}

	return nil
}
