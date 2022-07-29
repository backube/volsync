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
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/controller-runtime/pkg/client"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

type pvBackupCreate struct {
	// Cluster context name
	Cluster string
	// Namespace on Source cluster
	Namespace string
	// PVC to be backed up
	SourcePVC string
	// Name of the back up
	Name string
	// Name of the ReplicationSource object
	RSName string
	// Back up schedule
	schedule string
	// Restic configuration details
	resticConfig
	// Read in restic-config into stringData
	stringData map[string]string
	// Client object to communicate with a cluster
	client client.Client
	// Backup relationship object to be persisted to a config file
	pr *pvBackupRelationship
	// Backup retention policy
	retain retainPolicy
}

type retainPolicy struct {
	hourly  int32
	daily   int32
	weekly  int32
	monthly int32
	yearly  int32
	within  string
}

type resticConfig struct {
	viper.Viper
	filename string
}

// pvBackupCreateCmd represents the create command
var pvBackupCreateCmd = &cobra.Command{
	Use:   "create",
	Short: i18n.T("Create a new backup repo and build relationship resources"),
	Long: templates.LongDesc(i18n.T(`This command creates the necessary configuration
	inside the Cluster/Namespace, builds source CR and saves the details into
	to relationship file.

	ex: # kubectl volsync pv-backup create --relationship pvb1 --pvcname src/pv1
	--restic-config restic-conf.toml --name my-backup --cronspec "*/15 * * * *"
	
	NOTE: Example for restic-conf.toml can be found at
	"https://github.com/backube/volsync/tree/main/examples/restic/pv-backup/restic-conf.toml"`)),
	RunE: func(cmd *cobra.Command, args []string) error {
		pc, err := newPVBackupCreate(cmd)
		if err != nil {
			return err
		}
		return pc.Run(cmd.Context())
	},
}

func init() {
	initPVBackupCreateCmd(pvBackupCreateCmd)
}

func initPVBackupCreateCmd(pvBackupCreateCmd *cobra.Command) {
	pvBackupCmd.AddCommand(pvBackupCreateCmd)

	pvBackupCreateCmd.Flags().String("name", "", `name of the backup that can be used to 
	address backup & restore`)
	cobra.CheckErr(pvBackupCreateCmd.MarkFlagRequired("name"))
	pvBackupCreateCmd.Flags().String("restic-config", "", `path for the restic config file`)
	cobra.CheckErr(pvBackupCreateCmd.MarkFlagRequired("restic-config"))
	pvBackupCreateCmd.Flags().String("pvcname", "", "name of the PVC to backup: [context/]namespace/name")
	cobra.CheckErr(pvBackupCreateCmd.MarkFlagRequired("pvcname"))
	pvBackupCreateCmd.Flags().String("cronspec", "", "Cronspec describing the backup schedule")
}

func newPVBackupCreate(cmd *cobra.Command) (*pvBackupCreate, error) {
	pc := &pvBackupCreate{}
	// build struct pvBackupRelationship from cmd line args
	pr, err := newPVBackupRelationship(cmd)
	if err != nil {
		return nil, err
	}
	pc.pr = pr

	if err = pc.parseCLI(cmd); err != nil {
		return nil, err
	}

	return pc, nil
}

func (pc *pvBackupCreate) parseCLI(cmd *cobra.Command) error {
	pvcname, err := cmd.Flags().GetString("pvcname")
	if err != nil || pvcname == "" {
		return fmt.Errorf("failed to fetch the pvcname, err = %w", err)
	}
	xcr, err := ParseXClusterName(pvcname)
	if err != nil {
		return fmt.Errorf("failed to parse cluster name from pvcname, err = %w", err)
	}
	pc.SourcePVC = xcr.Name
	pc.Namespace = xcr.Namespace
	pc.Cluster = xcr.Cluster

	backupName, err := cmd.Flags().GetString("name")
	if err != nil {
		return fmt.Errorf("failed to fetch the backup name, err = %w", err)
	}
	pc.Name = backupName
	pc.RSName = backupName + "-backup-source"

	resticConfigFile, err := cmd.Flags().GetString("restic-config")
	if err != nil {
		return fmt.Errorf("failed to fetch the restic-config, err = %w", err)
	}
	resticConfig, err := parseResticConfig(resticConfigFile)
	if err != nil {
		return err
	}
	pc.resticConfig = *resticConfig

	stringData, err := parseSecretData(pc.resticConfig.Viper)
	if err != nil {
		return err
	}
	pc.stringData = stringData

	err = pc.userBackupRetainPolicy(pc.resticConfig.Viper)
	if err != nil {
		return err
	}

	cronSpec, err := cmd.Flags().GetString("cronspec")
	if err != nil {
		return fmt.Errorf("failed to fetch the cronspec, err = %w", err)
	}

	cs, err := parseCronSpec(cronSpec)
	if err != nil {
		return fmt.Errorf("failed to parse the cronspec, err = %w", err)
	}
	pc.schedule = *cs

	return nil
}

func (pc *pvBackupCreate) Run(ctx context.Context) error {
	k8sClient, err := newClient(pc.Cluster)
	if err != nil {
		return err
	}
	pc.client = k8sClient

	// Build struct pvBackupRelationshipSource from struct pvBackupCreate
	pc.pr.data.Source = pc.newPVBackupRelationshipSource()
	if err != nil {
		return err
	}

	// Add restic configurations into cluster
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pc.Name + "-source",
			Namespace: pc.Namespace,
		},
		StringData: pc.stringData,
	}
	err = createSecret(ctx, secret, pc.client)
	if err != nil {
		return err
	}

	// Creates the ReplicationSource if it doesn't exist
	_, err = pc.ensureReplicationSource(ctx)
	if err != nil {
		return err
	}

	// Wait for ReplicationSource
	_, err = pc.pr.data.waitForRSStatus(ctx, pc.client)
	if err != nil {
		return err
	}

	// Save the replication source details into relationship file
	if err = pc.pr.Save(); err != nil {
		return fmt.Errorf("unable to save relationship configuration: %w", err)
	}

	return nil
}

func (pc *pvBackupCreate) newPVBackupRelationshipSource() *pvBackupRelationshipSource {
	// Assign the values from pvBackupCreate built after parsing cmd args
	return &pvBackupRelationshipSource{
		Namespace: pc.Namespace,
		Cluster:   pc.Cluster,
		PVCName:   pc.SourcePVC,
		RSName:    pc.RSName,
		Trigger: volsyncv1alpha1.ReplicationSourceTriggerSpec{
			Schedule: &pc.schedule,
		},
		Source: volsyncv1alpha1.ReplicationSourceResticSpec{
			Repository: pc.Name + "-source",
			ReplicationSourceVolumeOptions: volsyncv1alpha1.ReplicationSourceVolumeOptions{
				CopyMethod: volsyncv1alpha1.CopyMethodClone,
			},
			Retain: &volsyncv1alpha1.ResticRetainPolicy{
				Hourly:  &pc.retain.hourly,
				Daily:   &pc.retain.daily,
				Weekly:  &pc.retain.weekly,
				Monthly: &pc.retain.monthly,
				Yearly:  &pc.retain.yearly,
				Within:  &pc.retain.within,
			},
		},
	}
}

func parseSecretData(v viper.Viper) (map[string]string, error) {
	mustKeys := []string{"RESTIC_REPOSITORY", "RESTIC_PASSWORD",
		"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY"}
	stringData := map[string]string{}

	for _, key := range mustKeys {
		value, ok := v.Get(key).(string)
		if !ok {
			return nil, fmt.Errorf("interface conversion: interface is not string, %w", os.ErrInvalid)
		}

		if value == "" {
			return nil, fmt.Errorf("mandatory value for key %s missing. %w", key, os.ErrNotExist)
		}
		stringData[key] = value
	}

	optKeys := []string{"AWS_DEFAULT_REGION", "ST_AUTH", "ST_USER", "ST_KEY", "OS_AUTH_URL",
		"OS_REGION_NAME", "OS_USERNAME", "OS_USER_ID", "OS_PASSWORD", "OS_TENANT_ID", "OS_TENANT_NAME",
		"OS_USER_DOMAIN_NAME", "OS_USER_DOMAIN_ID", "OS_PROJECT_NAME", "OS_PROJECT_DOMAIN_NAME",
		"OS_PROJECT_DOMAIN_ID", "OS_TRUST_ID", "OS_APPLICATION_CREDENTIAL_ID", "OS_APPLICATION_CREDENTIAL_NAME",
		"OS_APPLICATION_CREDENTIAL_SECRET", "OS_STORAGE_URL", "OS_AUTH_TOKEN", "B2_ACCOUNT_ID",
		"B2_ACCOUNT_KEY", "AZURE_ACCOUNT_NAME", "AZURE_ACCOUNT_KEY", "GOOGLE_PROJECT_ID", "GOOGLE_APPLICATION_CREDENTIALS"}

	for _, key := range optKeys {
		value, ok := v.Get(key).(string)
		if ok {
			stringData[key] = value
		}
	}

	return stringData, nil
}

func (pc *pvBackupCreate) userBackupRetainPolicy(v viper.Viper) error {
	keys := []string{"KEEP_HOURLY", "KEEP_DAILY", "KEEP_WEEKLY", "KEEP_MONTHLY",
		"KEEP_YEARLY", "KEEP_WITHIN"}
	for _, key := range keys {
		value, ok := v.Get(key).(string)
		if ok {
			if key == "KEEP_WITHIN" {
				pc.retain.within = value
				continue
			}
			keepValue, err := strconv.ParseInt(value, 10, 32)
			if err != nil {
				return fmt.Errorf("failed to build user backup retain policy, %w", err)
			}
			switch key {
			case "KEEP_HOURLY":
				pc.retain.hourly = int32(keepValue)
			case "KEEP_DAILY":
				pc.retain.daily = int32(keepValue)
			case "KEEP_WEEKLY":
				pc.retain.weekly = int32(keepValue)
			case "KEEP_MONTHLY":
				pc.retain.monthly = int32(keepValue)
			case "KEEP_YEARLY":
				pc.retain.yearly = int32(keepValue)
			}
		}
	}
	return nil
}

func (pc *pvBackupCreate) ensureReplicationSource(ctx context.Context) (
	*volsyncv1alpha1.ReplicationSource, error) {
	prs := pc.pr.data.Source

	rs := &volsyncv1alpha1.ReplicationSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      prs.RSName,
			Namespace: prs.Namespace,
		},
		Spec: volsyncv1alpha1.ReplicationSourceSpec{
			SourcePVC: prs.PVCName,
			Trigger:   &prs.Trigger,
			Restic:    &prs.Source,
		},
	}

	if err := pc.client.Create(ctx, rs); err != nil {
		return nil, err
	}
	klog.Infof("Created source CR: \"%s\" in Namespace: \"%s\" and Cluster: \"%s\"",
		rs.Name, rs.Namespace, pc.Cluster)

	return rs, nil
}

func (prd *pvBackupRelationshipData) waitForRSStatus(ctx context.Context, client client.Client) (
	*volsyncv1alpha1.ReplicationSource, error) {
	var (
		rs  *volsyncv1alpha1.ReplicationSource
		err error
	)
	klog.Infof("waiting for source CR to be available")
	err = wait.PollImmediate(5*time.Second, defaultRsyncKeyTimeout, func() (bool, error) {
		rs, err = prd.Source.getReplicationSource(ctx, client)
		if err != nil {
			return false, err
		}

		if rs.Status == nil || rs.Status.Conditions == nil {
			return false, nil
		}

		cond := apimeta.FindStatusCondition(rs.Status.Conditions, volsyncv1alpha1.ConditionSynchronizing)
		if cond == nil {
			klog.V(2).Infof("Waiting for backup source CR %s to be in Synchronizing state", rs.Name)
			return false, nil
		}
		if cond.Status == metav1.ConditionFalse || cond.Reason == volsyncv1alpha1.SynchronizingReasonError {
			return false, fmt.Errorf("backup source is in error condition, %s", cond.Message)
		}

		klog.V(2).Infof("pvbackup source CR is up")
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("backup source status: %w,", err)
	}

	return rs, nil
}
