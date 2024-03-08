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
	"text/template"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

type migrationHandlerRsyncTLS struct{}

var _ migrationHandler = &migrationHandlerRsyncTLS{}

type stunnelConfParams struct {
	StunnelConfFile    string
	StunnelPIDFile     string
	PSKFile            string
	LocalPort          int
	DestinationPort    int
	DestinationAddress string
}

func (mhrtls *migrationHandlerRsyncTLS) EnsureReplicationDestination(ctx context.Context, c client.Client,
	destConfig *migrationRelationshipDestinationV2) (*volsyncv1alpha1.ReplicationDestination, error) {
	rd := &volsyncv1alpha1.ReplicationDestination{
		ObjectMeta: metav1.ObjectMeta{
			Name:      destConfig.RDName,
			Namespace: destConfig.Namespace,
		},
		Spec: volsyncv1alpha1.ReplicationDestinationSpec{
			RsyncTLS: &volsyncv1alpha1.ReplicationDestinationRsyncTLSSpec{
				ReplicationDestinationVolumeOptions: volsyncv1alpha1.ReplicationDestinationVolumeOptions{
					DestinationPVC: &destConfig.PVCName,
					CopyMethod:     destConfig.CopyMethod,
				},
				ServiceType: destConfig.ServiceType,
				MoverConfig: volsyncv1alpha1.MoverConfig{
					MoverSecurityContext: destConfig.MoverSecurityContext,
				},
			},
		},
	}
	if err := c.Create(ctx, rd); err != nil {
		return nil, err
	}
	klog.Infof("Created ReplicationDestination: \"%s\" in Namespace: \"%s\"",
		rd.Name, rd.Namespace)

	rd.Spec.RsyncTLS.MoverSecurityContext = destConfig.MoverSecurityContext

	return rd, nil
}

//nolint:dupl
func (mhrtls *migrationHandlerRsyncTLS) WaitForRDStatus(ctx context.Context, c client.Client,
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

			if rd.Status.RsyncTLS == nil {
				return false, nil
			}

			if rd.Status.RsyncTLS.Address == nil {
				klog.V(2).Infof("Waiting for MigrationDestination %s RSyncTLS address to populate", rd.GetName())
				return false, nil
			}

			if rd.Status.RsyncTLS.KeySecret == nil {
				klog.V(2).Infof("Waiting for MigrationDestination %s RSyncTLS keySecret to populate", rd.GetName())
				return false, nil
			}

			klog.V(2).Infof("Found MigrationDestination RSyncTLS Address: %s", *rd.Status.RsyncTLS.Address)
			return true, nil
		})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch rd status: %w,", err)
	}

	return rd, nil
}

func (mhrtls *migrationHandlerRsyncTLS) RunMigration(ctx context.Context, c client.Client,
	source string, destConfig *migrationRelationshipDestinationV2) error {
	var stunnelTempDir *string
	var destAddr string
	var err error
	defer func() {
		// Remove the directory containing secrets
		if stunnelTempDir != nil {
			if err = os.RemoveAll(*stunnelTempDir); err != nil {
				klog.Infof("failed to remove temporary directory with stunnel conf (%s): %v",
					*stunnelTempDir, err)
			}
			klog.Infof("removed temp dir: %s", *stunnelTempDir)
		}
	}()

	tempDir, err := os.MkdirTemp("", "stunnelConf")
	if err != nil {
		return fmt.Errorf("unable to create temporary directory %w", err)
	}
	stunnelTempDir = &tempDir
	klog.Infof("created temp dir: %s", *stunnelTempDir)

	pskFile := filepath.Join(*stunnelTempDir, "psk.txt")
	// Write pre-shared key to psk file and find dest addr
	destAddr, err = mhrtls.retrieveSecretsAndDestAddr(ctx, c, pskFile, destConfig)
	if err != nil {
		return err
	}

	stunnelParams := stunnelConfParams{
		StunnelConfFile:    filepath.Join(*stunnelTempDir, "stunnel-client.conf"),
		StunnelPIDFile:     filepath.Join(*stunnelTempDir, "stunnel-client.pid"),
		PSKFile:            pskFile,
		LocalPort:          defaultLocalStunnelPort,       //TODO: allow to config from cmd line
		DestinationPort:    defaultDestinationStunnelPort, //TODO: allow to config from cmd line
		DestinationAddress: destAddr,
	}

	err = mhrtls.createStunnelConf(stunnelParams)
	if err != nil {
		return err
	}

	// Do rsync
	err = mhrtls.runRsyncTLS(ctx, source, stunnelParams, destConfig)
	if err != nil {
		return err
	}

	return nil
}

func (mhrtls *migrationHandlerRsyncTLS) retrieveSecretsAndDestAddr(ctx context.Context, c client.Client,
	pskFile string, destConfig *migrationRelationshipDestinationV2) (string, error) {
	klog.Infof("Extracting ReplicationDestination secrets")

	rd := &volsyncv1alpha1.ReplicationDestination{
		ObjectMeta: metav1.ObjectMeta{
			Name:      destConfig.RDName,
			Namespace: destConfig.Namespace,
		},
	}
	_, err := mhrtls.WaitForRDStatus(ctx, c, rd)
	if err != nil {
		return "", err
	}

	destAddr := *rd.Status.RsyncTLS.Address
	keySecretName := rd.Status.RsyncTLS.KeySecret
	keySecret := &corev1.Secret{}
	nsName := types.NamespacedName{
		Namespace: destConfig.Namespace,
		Name:      *keySecretName,
	}
	err = c.Get(ctx, nsName, keySecret)
	if err != nil {
		return "", fmt.Errorf("error retrieving destination keySecret %s: %w", *keySecretName, err)
	}

	err = os.WriteFile(pskFile, keySecret.Data["psk.txt"], 0600)
	if err != nil {
		return "", fmt.Errorf("unable to write to the file, %w", err)
	}

	return destAddr, nil
}

// Writes out a stunnel conf file to the path in stunnelParams.StunnelConfFile
func (mhrtls *migrationHandlerRsyncTLS) createStunnelConf(stunnelParams stunnelConfParams) error {
	f, err := os.Create(stunnelParams.StunnelConfFile)
	if err != nil {
		return err
	}
	defer f.Close()

	// Note this differs from mover-rsync-tls client.sh in that we run in the foreground
	stunnelConfTemplate := `
; Global options
debug = debug
foreground = yes
output = /dev/stdout
pid = {{ .StunnelPIDFile }}
socket = l:SO_KEEPALIVE=1
socket = l:TCP_KEEPIDLE=180
socket = r:SO_KEEPALIVE=1
socket = r:TCP_KEEPIDLE=180
syslog = no

[rsync]
ciphers = PSK
PSKsecrets = {{ .PSKFile }}
; Port to listen for incoming connection from rsync
accept = 127.0.0.1:{{ .LocalPort }}
; We are the client
client = yes
connect = {{ .DestinationAddress }}:{{ .DestinationPort }}
`

	t, err := template.New("stunnelconf").Parse(stunnelConfTemplate)
	if err != nil {
		return err
	}

	err = t.Execute(f, stunnelParams)
	if err != nil {
		return err
	}

	return nil
}

//nolint:funlen
func (mhrtls *migrationHandlerRsyncTLS) runRsyncTLS(ctx context.Context, source string,
	stunnelParams stunnelConfParams, destConfig *migrationRelationshipDestinationV2) error {
	stunnelContext, stunnelCancel := context.WithCancel(ctx)
	//nolint:gosec
	stunnelCmd := exec.CommandContext(stunnelContext, "stunnel", stunnelParams.StunnelConfFile)
	defer func() {
		stunnelCancel()
		_ = stunnelCmd.Wait() // Ignore errors, the tunnel will show error because we killed it via context
		klog.Info("stunnel shutdown complete.")
	}()

	stunnelCmd.Stderr = os.Stderr
	// stunnel in foreground mode will also log everything to stderr so don't bother with stdout
	//stunnelCmd.Stdout = os.Stdout
	klog.Infof("Starting local stunnel listening on port %d", stunnelParams.LocalPort)
	err := stunnelCmd.Start()
	if err != nil {
		return fmt.Errorf("failed to run =%w", err)
	}

	// Make sure stunnel has started
	const maxRetries = 20
	pidFileExists := false
	for i := 0; i < maxRetries; i++ {
		time.Sleep(1 * time.Second)
		_, err = os.Stat(stunnelParams.StunnelPIDFile)
		if err == nil {
			pidFileExists = true
			break
		}
	}
	if !pidFileExists {
		return fmt.Errorf("stunnel failed to start - pid file %s not found", stunnelParams.StunnelPIDFile)
	}

	// Now run rsync
	// 1st run preserves as much as possible, but excludes the root directory
	localRsyncTunnelAddr := fmt.Sprintf("rsync://127.0.0.1:%d/data", stunnelParams.LocalPort)

	cmdArgs := []string{
		"-aAhHSxz",
		"--exclude=lost+found",
		"--itemize-changes",
		"--info=stats2,misc2",
	}

	sourcePath := filepath.Clean(source) // Will remove any trailing slash
	// filepath.Glob() includes hidden files (files that start with .) but does not include . or ..
	sourceFiles, err := filepath.Glob(sourcePath + "/*")
	if err != nil {
		return err
	}

	cmdArgs = append(cmdArgs, sourceFiles...)
	cmdArgs = append(cmdArgs, localRsyncTunnelAddr)

	//nolint:gosec
	cmd := exec.CommandContext(ctx, "rsync", cmdArgs...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	klog.Infof("Migrating Data from \"%s\" to \"%s\\%s\\%s\"", source, destConfig.Cluster,
		destConfig.Namespace, destConfig.PVCName)
	err = cmd.Start()
	if err != nil {
		return fmt.Errorf("failed to run =%w", err)
	}
	err = cmd.Wait()
	if err != nil {
		return fmt.Errorf("command finished with an error, %w", err)
	}

	// rsync - 2nd run
	// To delete extra files, must sync at the directory-level, but need to avoid
	// trying to modify the directory itself. This pass will only delete files
	// that exist on the destination but not on the source, not make updates.

	//nolint:gosec
	cmd2 := exec.CommandContext(ctx, "rsync", "-rx", "--exclude=lost+found",
		"--ignore-existing", "--ignore-non-existing", "--delete",
		"--itemize-changes", "--info=stats2,misc2", sourcePath+"/", localRsyncTunnelAddr)
	cmd2.Stderr = os.Stderr
	cmd2.Stdout = os.Stdout
	klog.Infof("\n2nd rsync to clean up extra files at dest...")
	err = cmd2.Start()
	if err != nil {
		return fmt.Errorf("failed to run =%w", err)
	}
	err = cmd2.Wait()
	if err != nil {
		return fmt.Errorf("command finished with an error, %w", err)
	}

	return nil
}
