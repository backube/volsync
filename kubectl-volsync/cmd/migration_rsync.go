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
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type migrationSync struct {
	mr *migrationRelationship
	// Address is the remote address to connect to for replication.
	Address string
	// Source volume to be migrated
	Source string
	// client object to communicate with a cluster
	client client.Client
}

// migrationCreateCmd represents the create command
var migrationSyncCmd = &cobra.Command{
	Use:   "rsync",
	Short: i18n.T("Rsync data from source to destination"),
	Long: templates.LongDesc(i18n.T(`
	This command ensures the migration of data from source to destination
	via rsync over ssh. The execution of this command should be followed by
	migration create which establishes the relationship.
	`)),
	RunE: func(cmd *cobra.Command, args []string) error {
		ms := &migrationSync{}
		mr, err := loadMigrationRelationship(cmd)
		if err != nil {
			return err
		}
		ms.mr = mr

		err = ms.newMigrationSync(cmd)
		if err != nil {
			return err
		}

		return ms.Run(cmd.Context())
	},
}

func init() {
	initmigrationSyncCmd(migrationSyncCmd)
}

func initmigrationSyncCmd(migrationCreateCmd *cobra.Command) {
	migrationCmd.AddCommand(migrationCreateCmd)

	migrationCreateCmd.Flags().String("source", "", "source volume to be migrated")
	cobra.CheckErr(migrationCreateCmd.MarkFlagRequired("source"))
}

func (ms *migrationSync) Run(ctx context.Context) error {
	mrd := ms.mr.data.Destination
	k8sClient, err := newClient(mrd.Cluster)
	if err != nil {
		return err
	}
	ms.client = k8sClient

	// Ensure source volume
	_, err = os.Stat(ms.Source)
	if err != nil {
		return fmt.Errorf("failed to access the source volume, %w", err)
	}

	var sshKeyDir *string
	defer func() {
		// Remove the directory containing secrets
		if sshKeyDir != nil {
			if err = os.RemoveAll(*sshKeyDir); err != nil {
				klog.Infof("failed to remove temporary directory with ssh keys (%s): %w",
					sshKeyDir, err)
			}
		}
	}()
	// Retrieve Secrets/keys
	sshKeyDir, err = ms.retrieveSecrets(ctx)
	if err != nil {
		return err
	}

	// Do rysnc
	err = ms.runRsync(ctx, *sshKeyDir)
	if err != nil {
		return err
	}

	return nil
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

func (ms *migrationSync) newMigrationSync(cmd *cobra.Command) error {
	source, err := cmd.Flags().GetString("source")
	if err != nil || source == "" {
		return fmt.Errorf("failed to fetch the source arg, err = %w", err)
	}
	ms.Source = source

	return nil
}

//nolint:funlen
func (ms *migrationSync) retrieveSecrets(ctx context.Context) (*string, error) {
	klog.Infof("Extracting ReplicationDestination secrets")
	mrd := ms.mr.data.Destination
	rd, err := mrd.waitForRDStatus(ctx, ms.client)
	if err != nil {
		return nil, err
	}
	ms.Address = *rd.Status.Rsync.Address
	sshKeysSecret := rd.Status.Rsync.SSHKeys
	sshSecret := &corev1.Secret{}
	nsName := types.NamespacedName{
		Namespace: mrd.Namespace,
		Name:      *sshKeysSecret,
	}
	err = ms.client.Get(ctx, nsName, sshSecret)
	if err != nil {
		return nil, fmt.Errorf("error retrieving destination sshSecret %s: %w", *sshKeysSecret, err)
	}

	sshKeydir, err := ioutil.TempDir("", "sshkeys")
	if err != nil {
		return nil, fmt.Errorf("unable to create temporary directory %w", err)
	}

	filename := filepath.Join(sshKeydir, "source")
	err = ioutil.WriteFile(filename, sshSecret.Data["source"], 0600)
	if err != nil {
		return &sshKeydir, fmt.Errorf("unable to write to the file, %w", err)
	}

	filename = filepath.Join(sshKeydir, "source.pub")
	err = ioutil.WriteFile(filename, sshSecret.Data["source.pub"], 0600)
	if err != nil {
		return &sshKeydir, fmt.Errorf("unable to write to the file, %w", err)
	}

	filename = filepath.Join(sshKeydir, "destination.pub")
	err = ioutil.WriteFile(filename, sshSecret.Data["destination.pub"], 0600)
	if err != nil {
		return &sshKeydir, fmt.Errorf("unable to write to the file, %w", err)
	}

	return &sshKeydir, nil
}

func (ms *migrationSync) runRsync(ctx context.Context, keydir string) error {
	bin := "rsync"
	sshKey := keydir + "/source"
	ssh := "ssh -i " + sshKey
	dest := "root@" + ms.Address + ":."

	cmd := exec.CommandContext(ctx, bin, "-aAhHSxze", ssh, "--delete",
		"--itemize-changes", "--info=stats2,misc2", ms.Source, dest)

	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	klog.Infof("Executing \"%v\"", cmd)
	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("failed to run =%w", err)
	}
	err = cmd.Wait()
	if err != nil {
		return fmt.Errorf("command finished with an error, %w", err)
	}

	return nil
}
