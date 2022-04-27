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
	cron "github.com/robfig/cron/v3"
	"github.com/spf13/cobra"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
)

type pvBackupSchedule struct {
	// Parsed CLI options
	schedule string
	pr       *pvBackupRelationship
}

// pvBackupScheduleCmd represents the pvBackupSchedule command
var pvBackupScheduleCmd = &cobra.Command{
	Use:   "schedule",
	Short: i18n.T("Apply new backup schedule on the existing relationship resources"),
	Long: templates.LongDesc(i18n.T(`
	This command schedules/reschedules backup instances.

	ex: # kubectl volsync pv-backup schedule --cronspec "*/30 * * * *" --relationship pvb1
	`)),
	RunE: func(cmd *cobra.Command, args []string) error {
		ps, err := newPVBackupSchedule(cmd)
		if err != nil {
			return err
		}
		pr, err := loadPVBackupRelationship(cmd)
		if err != nil {
			return err
		}
		ps.pr = pr
		return ps.Run(cmd.Context())
	},
}

func init() {
	initPVBackupScheduleCmd(pvBackupScheduleCmd)
}

func initPVBackupScheduleCmd(pvBackupScheduleCmd *cobra.Command) {
	pvBackupCmd.AddCommand(pvBackupScheduleCmd)
	pvBackupScheduleCmd.Flags().String("cronspec", "", "Cronspec describing the backup schedule")
	cobra.CheckErr(pvBackupScheduleCmd.MarkFlagRequired("cronspec"))
}

func newPVBackupSchedule(cmd *cobra.Command) (*pvBackupSchedule, error) {
	// build struct pvBackupRelationship from cmd line args
	ps := &pvBackupSchedule{}

	cs, err := cmd.Flags().GetString("cronspec")
	if err != nil {
		return ps, err
	}
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	if _, err = parser.Parse(cs); err != nil {
		return ps, err
	}
	ps.schedule = cs
	return ps, nil
}

func (ps *pvBackupSchedule) Run(ctx context.Context) error {
	client, err := newClient(ps.pr.data.Source.Cluster)
	if err != nil {
		return err
	}

	ps.pr.data.Source.Trigger = volsyncv1alpha1.ReplicationSourceTriggerSpec{
		Schedule: &ps.schedule,
	}

	if err := ps.pr.Apply(ctx, client); err != nil {
		return err
	}
	if err := ps.pr.Save(); err != nil {
		return fmt.Errorf("unable to save relationship configuration: %w", err)
	}

	return nil
}
