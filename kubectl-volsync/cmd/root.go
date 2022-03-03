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
	goflag "flag"
	"os"
	"path"
	"strings"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/spf13/cobra"
	"k8s.io/component-base/logs"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
)

const (
	defaultRsyncKeyTimeout   = 10 * time.Minute
	defaultVolumeSyncTimeout = 30 * time.Minute
)

// volsyncVersion value is set at build time via ldflags
var volsyncVersion = "0.0.0"

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "volsync",
	Short: i18n.T("A kubectl plugin to interact with the VolSync operator"),
	Long: templates.LongDesc(i18n.T(`
	This plugin can be used to configure replication relationships using the
	VolSync operator.

	The plugin has a number of sub-commands that are organized based on common
	data movement tasks such as:

	* Creating a cross-cluster data replication relationship
	* Migrating data into a Kubernetes cluster
	* Establishing a simple PV backup schedule
	`)),
	// Use Cobra's built-in version command
	Version: volsyncVersion,
}

// Execute adds all child commands to the root command and sets flags
// appropriately. This is called by main.main(). It only needs to happen once to
// the rootCmd.
func Execute() {
	// Cobra doesn't have a way to specify a two word command (ie. "kubectl krew"), so set a custom usage template
	// with kubectl in it. Cobra will use this template for the root and all child commands.
	// Taken from
	// https://github.com/kubernetes-sigs/krew/blob/fd53697d5e5ee18138df50088037c9715cc50214/cmd/krew/cmd/root.go#L104-L108
	rootCmd.SetUsageTemplate(strings.NewReplacer(
		"{{.UseLine}}", "kubectl {{.UseLine}}",
		"{{.CommandPath}}", "kubectl {{.CommandPath}}").Replace(rootCmd.UsageTemplate()))
	logs.InitLogs()
	defer logs.FlushLogs()
	cobra.CheckErr(rootCmd.Execute())
}

func init() {
	configDirDefault := ""
	if home, err := os.UserHomeDir(); err == nil {
		configDirDefault = path.Join(home, ".volsync")
	}
	rootCmd.PersistentFlags().String("config-dir", configDirDefault,
		"directory for VolSync config files")
	// Add flags that are set by other packages:
	// - controller-runtime/pkg/client/config provides kubeconfig
	// - logging packages provide config flags for logs
	rootCmd.PersistentFlags().AddGoFlagSet(goflag.CommandLine)
}
