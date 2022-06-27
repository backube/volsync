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
	"os"
	"time"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type pvBackupSync struct {
	pr *pvBackupRelationship
}

var pvBackupSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: i18n.T("Run single manual backup"),
	Long: templates.LongDesc(i18n.T(`
	This command triggers the single backup instance and waits till backup to complete.

	ex: kubectl volsync pv-backup sync --relationship pvb1

	NOTE: If user issues sync command, the backup schedule will be cleared. If user wants
	to continue the schedule, one should issue schedule command and set the schedule for
	backup

	`)),
	RunE: func(cmd *cobra.Command, args []string) error {
		ps, err := newPVBackupSync(cmd)
		if err != nil {
			return err
		}
		pr, err := loadPVBackupRelationship(cmd)
		if err != nil {
			return err
		}
		if pr.data.Source == nil {
			return fmt.Errorf("incompatible relationship, %w", os.ErrInvalid)
		}
		ps.pr = pr

		return ps.Run(cmd.Context())
	},
}

func init() {
	initPVBackupSyncCmd(pvBackupSyncCmd)
}

func initPVBackupSyncCmd(pvBackupSyncCmd *cobra.Command) {
	pvBackupCmd.AddCommand(pvBackupSyncCmd)
}

func newPVBackupSync(cmd *cobra.Command) (*pvBackupSync, error) {
	return &pvBackupSync{}, nil
}

func (ps *pvBackupSync) Run(ctx context.Context) error {
	client, err := newClient(ps.pr.data.Source.Cluster)
	if err != nil {
		return err
	}

	ps.pr.data.Source.Trigger = volsyncv1alpha1.ReplicationSourceTriggerSpec{
		Manual: time.Now().Format(time.RFC3339),
	}

	if err := ps.pr.Apply(ctx, client); err != nil {
		return err
	}

	if err := ps.pr.Save(); err != nil {
		return fmt.Errorf("unable to save relationship configuration: %w", err)
	}

	return ps.waitForSync(ctx, client)
}

func (ps *pvBackupSync) waitForSync(ctx context.Context, srcClient client.Client) error {
	klog.Infof("waiting for synchronization to complete")
	rs := volsyncv1alpha1.ReplicationSource{}
	rsName := types.NamespacedName{
		Name:      ps.pr.data.Source.RSName,
		Namespace: ps.pr.data.Source.Namespace,
	}

	return waitForSync(ctx, srcClient, rsName, rs)
}
