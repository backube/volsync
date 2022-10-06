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
	DestAddr string
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
		ms, err := newMigrationSync(cmd)
		if err != nil {
			return err
		}
		mr, err := loadMigrationRelationship(cmd)
		if err != nil {
			return err
		}
		ms.mr = mr

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
	k8sClient, err := newClient(ms.mr.data.Destination.Cluster)
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

func newMigrationSync(cmd *cobra.Command) (*migrationSync, error) {
	ms := &migrationSync{}
	source, err := cmd.Flags().GetString("source")
	if err != nil || source == "" {
		return nil, fmt.Errorf("failed to fetch the source arg, err = %w", err)
	}
	ms.Source = source

	return ms, nil
}

//nolint:funlen
func (ms *migrationSync) retrieveSecrets(ctx context.Context) (*string, error) {
	klog.Infof("Extracting ReplicationDestination secrets")
	mrd := ms.mr.data.Destination
	rd, err := mrd.waitForRDStatus(ctx, ms.client)
	if err != nil {
		return nil, err
	}
	ms.DestAddr = *rd.Status.Rsync.Address
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

	sshKeydir, err := os.MkdirTemp("", "sshkeys")
	if err != nil {
		return nil, fmt.Errorf("unable to create temporary directory %w", err)
	}

	filename := filepath.Join(sshKeydir, "source")
	err = os.WriteFile(filename, sshSecret.Data["source"], 0600)
	if err != nil {
		return &sshKeydir, fmt.Errorf("unable to write to the file, %w", err)
	}

	filename = filepath.Join(sshKeydir, "source.pub")
	err = os.WriteFile(filename, sshSecret.Data["source.pub"], 0600)
	if err != nil {
		return &sshKeydir, fmt.Errorf("unable to write to the file, %w", err)
	}

	filename = filepath.Join(sshKeydir, "destination.pub")
	destinationPub := fmt.Sprintf("%s %s", ms.DestAddr,
		sshSecret.Data["destination.pub"])
	err = os.WriteFile(filename, []byte(destinationPub), 0600)
	if err != nil {
		return &sshKeydir, fmt.Errorf("unable to write to the file, %w", err)
	}

	return &sshKeydir, nil
}

func (ms *migrationSync) runRsync(ctx context.Context, sshKeydir string) error {
	sshKey := filepath.Join(sshKeydir, "source")
	knownHostfile := filepath.Join(sshKeydir, "destination.pub")
	ssh := fmt.Sprintf("ssh -i %s -o UserKnownHostsFile=%s -o StrictHostKeyChecking=yes",
		sshKey, knownHostfile)
	dest := fmt.Sprintf("root@%s:.", ms.DestAddr)

	cmd := exec.CommandContext(ctx, "rsync", "-aAhHSxze", ssh, "--delete",
		"--itemize-changes", "--info=stats2,misc2", ms.Source, dest)

	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	klog.Infof("Migrating Data from \"%s\" to \"%s\\%s\\%s\"", ms.Source, ms.mr.data.Destination.Cluster,
		ms.mr.data.Destination.Namespace, ms.mr.data.Destination.PVCName)
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
