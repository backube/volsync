/*
Copyright © 2024 The VolSync authors

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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

type migrationHandlerRsync struct{}

var _ migrationHandler = &migrationHandlerRsync{}

func (mhr *migrationHandlerRsync) EnsureReplicationDestination(ctx context.Context, c client.Client,
	destConfig *migrationRelationshipDestinationV2) (*volsyncv1alpha1.ReplicationDestination, error) {
	rd := &volsyncv1alpha1.ReplicationDestination{
		ObjectMeta: metav1.ObjectMeta{
			Name:      destConfig.RDName,
			Namespace: destConfig.Namespace,
		},
		Spec: volsyncv1alpha1.ReplicationDestinationSpec{
			Rsync: &volsyncv1alpha1.ReplicationDestinationRsyncSpec{
				ReplicationDestinationVolumeOptions: volsyncv1alpha1.ReplicationDestinationVolumeOptions{
					DestinationPVC: &destConfig.PVCName,
					CopyMethod:     destConfig.CopyMethod,
				},
				ServiceType: destConfig.ServiceType,
			},
		},
	}
	if err := c.Create(ctx, rd); err != nil {
		return nil, err
	}
	klog.Infof("Created ReplicationDestination: \"%s\" in Namespace: \"%s\"",
		rd.Name, rd.Namespace)

	return rd, nil
}

//nolint:dupl
func (mhr *migrationHandlerRsync) WaitForRDStatus(ctx context.Context, c client.Client,
	rd *volsyncv1alpha1.ReplicationDestination) (*volsyncv1alpha1.ReplicationDestination, error) {
	// wait for migrationdestination to become ready
	klog.Infof("waiting for keySecret & address of destination to be available")
	err := wait.PollUntilContextTimeout(ctx, 5*time.Second, defaultRsyncKeyTimeout, true, /*immediate*/
		func(ctx context.Context) (bool, error) {
			err := c.Get(ctx, client.ObjectKeyFromObject(rd), rd)
			if err != nil {
				return false, err
			}
			if rd.Status == nil {
				return false, nil
			}

			if rd.Status.Rsync == nil {
				return false, nil
			}

			if rd.Status.Rsync.Address == nil {
				klog.V(2).Infof("Waiting for MigrationDestination %s RSync address to populate", rd.GetName())
				return false, nil
			}

			if rd.Status.Rsync.SSHKeys == nil {
				klog.V(2).Infof("Waiting for MigrationDestination %s RSync sshkeys to populate", rd.GetName())
				return false, nil
			}

			klog.V(2).Infof("Found MigrationDestination RSync Address: %s", *rd.Status.Rsync.Address)
			return true, nil
		})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch rd status: %w,", err)
	}

	return rd, nil
}

func (mhr *migrationHandlerRsync) RunMigration(ctx context.Context, c client.Client,
	source string, destConfig *migrationRelationshipDestinationV2) error {
	var sshKeyDir *string
	var destAddr string
	var err error
	defer func() {
		// Remove the directory containing secrets
		if sshKeyDir != nil {
			if err = os.RemoveAll(*sshKeyDir); err != nil {
				klog.Infof("failed to remove temporary directory with ssh keys (%s): %v",
					*sshKeyDir, err)
			}
		}
	}()
	// Retrieve Secrets/keys
	sshKeyDir, destAddr, err = mhr.retrieveSecretsAndDestAddr(ctx, c, destConfig)
	if err != nil {
		return err
	}

	// Do rsync
	err = mhr.runRsync(ctx, source, *sshKeyDir, destAddr, destConfig)
	if err != nil {
		return err
	}

	return nil
}

func (mhr *migrationHandlerRsync) retrieveSecretsAndDestAddr(ctx context.Context, c client.Client,
	destConfig *migrationRelationshipDestinationV2) (*string, string, error) {
	klog.Infof("Extracting ReplicationDestination secrets")

	rd := &volsyncv1alpha1.ReplicationDestination{
		ObjectMeta: metav1.ObjectMeta{
			Name:      destConfig.RDName,
			Namespace: destConfig.Namespace,
		},
	}
	_, err := mhr.WaitForRDStatus(ctx, c, rd)
	if err != nil {
		return nil, "", err
	}

	destAddr := *rd.Status.Rsync.Address
	sshKeysSecret := rd.Status.Rsync.SSHKeys
	sshSecret := &corev1.Secret{}
	nsName := types.NamespacedName{
		Namespace: destConfig.Namespace,
		Name:      *sshKeysSecret,
	}
	err = c.Get(ctx, nsName, sshSecret)
	if err != nil {
		return nil, "", fmt.Errorf("error retrieving destination sshSecret %s: %w", *sshKeysSecret, err)
	}

	sshKeydir, err := os.MkdirTemp("", "sshkeys")
	if err != nil {
		return nil, "", fmt.Errorf("unable to create temporary directory %w", err)
	}

	filename := filepath.Join(sshKeydir, "source")
	err = os.WriteFile(filename, sshSecret.Data["source"], 0600)
	if err != nil {
		return &sshKeydir, "", fmt.Errorf("unable to write to the file, %w", err)
	}

	filename = filepath.Join(sshKeydir, "source.pub")
	err = os.WriteFile(filename, sshSecret.Data["source.pub"], 0600)
	if err != nil {
		return &sshKeydir, "", fmt.Errorf("unable to write to the file, %w", err)
	}

	filename = filepath.Join(sshKeydir, "destination.pub")
	destinationPub := fmt.Sprintf("%s %s", destAddr, sshSecret.Data["destination.pub"])
	err = os.WriteFile(filename, []byte(destinationPub), 0600)
	if err != nil {
		return &sshKeydir, "", fmt.Errorf("unable to write to the file, %w", err)
	}

	return &sshKeydir, destAddr, nil
}

func (mhr *migrationHandlerRsync) runRsync(ctx context.Context, source string,
	sshKeydir string, destAddr string, destConfig *migrationRelationshipDestinationV2) error {
	sshKey := filepath.Join(sshKeydir, "source")
	knownHostfile := filepath.Join(sshKeydir, "destination.pub")
	ssh := fmt.Sprintf("ssh -i %s -o UserKnownHostsFile=%s -o StrictHostKeyChecking=yes",
		sshKey, knownHostfile)
	dest := fmt.Sprintf("root@%s:.", destAddr)

	cmd := exec.CommandContext(ctx, "rsync", "-aAhHSxze", ssh, "--delete",
		"--itemize-changes", "--info=stats2,misc2", source, dest)

	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	klog.Infof("Migrating Data from \"%s\" to \"%s\\%s\\%s\"", source, destConfig.Cluster,
		destConfig.Namespace, destConfig.PVCName)
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
