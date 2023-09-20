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
	"time"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/controller-runtime/pkg/client"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

type replicationSync struct {
	rel *replicationRelationship
}

// replicationSyncCmd represents the replicationSync command
var replicationSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: i18n.T("Run a single synchronization"),
	Long: templates.LongDesc(i18n.T(`
	This command causes a one-time synchronization. Use the "schedule" command
	for scheduled replication.
	`)),
	RunE: func(cmd *cobra.Command, args []string) error {
		rsync, err := newReplicationSync(cmd)
		if err != nil {
			return err
		}
		rsync.rel, err = loadReplicationRelationship(cmd)
		if err != nil {
			return err
		}
		return rsync.Run(cmd.Context())
	},
}

func init() {
	replicationCmd.AddCommand(replicationSyncCmd)
}

func newReplicationSync(_ *cobra.Command) (*replicationSync, error) {
	return &replicationSync{}, nil
}

func (rs *replicationSync) Run(ctx context.Context) error {
	srcClient, dstClient, _ := rs.rel.GetClients()

	if rs.rel.data.Source == nil {
		return fmt.Errorf("please use \"replication set-source\" before triggering a synchronization")
	}

	rs.rel.data.Source.Trigger = volsyncv1alpha1.ReplicationSourceTriggerSpec{
		Manual: time.Now().Format(time.RFC3339),
	}

	if err := rs.rel.Apply(ctx, srcClient, dstClient); err != nil {
		return err
	}
	if err := rs.rel.Save(); err != nil {
		return fmt.Errorf("unable to save relationship configuration: %w", err)
	}

	return rs.waitForSync(ctx, srcClient)
}

func (rs *replicationSync) waitForSync(ctx context.Context, srcClient client.Client) error {
	klog.Infof("waiting for synchronization to complete")
	rsrc := volsyncv1alpha1.ReplicationSource{}
	rsName := types.NamespacedName{
		Name:      rs.rel.data.Source.RSName,
		Namespace: rs.rel.data.Source.Namespace,
	}
	err := wait.PollUntilContextTimeout(ctx, 5*time.Second, defaultVolumeSyncTimeout, true, /*immediate*/
		func(ctx context.Context) (bool, error) {
			if err := srcClient.Get(ctx, rsName, &rsrc); err != nil {
				return false, err
			}
			if rsrc.Spec.Trigger == nil || rsrc.Spec.Trigger.Manual == "" {
				return false, fmt.Errorf("internal error: manual trigger not specified")
			}
			if rsrc.Status == nil {
				return false, nil
			}
			if rsrc.Status.LastManualSync != rsrc.Spec.Trigger.Manual {
				return false, nil
			}
			return true, nil
		})
	return err
}
