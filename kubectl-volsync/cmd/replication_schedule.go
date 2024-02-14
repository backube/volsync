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

	cron "github.com/robfig/cron/v3"
	"github.com/spf13/cobra"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

type replicationSchedule struct {
	rel *replicationRelationship
	// Parsed CLI options
	schedule string
}

// replicationScheduleCmd represents the replicationSchedule command
var replicationScheduleCmd = &cobra.Command{
	Use:   "schedule",
	Short: i18n.T("Set replication schedule for the relationship"),
	Long: templates.LongDesc(i18n.T(`
	This command sets the schedule for replicating data.
	`)),
	RunE: func(cmd *cobra.Command, _ []string) error {
		rsched, err := newReplicationSchedule(cmd)
		if err != nil {
			return err
		}
		rsched.rel, err = loadReplicationRelationship(cmd)
		if err != nil {
			return err
		}
		return rsched.Run(cmd.Context())
	},
}

func init() {
	replicationCmd.AddCommand(replicationScheduleCmd)

	replicationScheduleCmd.Flags().String("cronspec", "", "Cronspec describing the replication schedule")
	cobra.CheckErr(replicationScheduleCmd.MarkFlagRequired("cronspec"))
}

func newReplicationSchedule(cmd *cobra.Command) (*replicationSchedule, error) {
	// Ensure the cronspec is parsable, but we don't actually care what it parses into.
	cs, err := cmd.Flags().GetString("cronspec")
	if err != nil {
		return nil, err
	}
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	if _, err = parser.Parse(cs); err != nil {
		return nil, err
	}

	return &replicationSchedule{
		schedule: cs,
	}, nil
}

func (rs *replicationSchedule) Run(ctx context.Context) error {
	srcClient, dstClient, _ := rs.rel.GetClients()

	if rs.rel.data.Source == nil {
		return fmt.Errorf("please use \"replication set-source\" prior to setting the replication schedule")
	}

	rs.rel.data.Source.Trigger = volsyncv1alpha1.ReplicationSourceTriggerSpec{
		Schedule: &rs.schedule,
	}

	if err := rs.rel.Apply(ctx, srcClient, dstClient); err != nil {
		return err
	}
	if err := rs.rel.Save(); err != nil {
		return fmt.Errorf("unable to save relationship configuration: %w", err)
	}

	return nil
}
