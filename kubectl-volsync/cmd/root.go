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
	"os"
	"path"

	"github.com/spf13/cobra"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
)

// volsyncVersion value is set at build time via ldflags
var volsyncVersion = "0.0.0"

var configDir string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "kubectl-volsync",
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
	Version: volsyncVersion,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	setupConfigDir()
	cobra.CheckErr(rootCmd.Execute())
}

func init() {
	rootCmd.PersistentFlags().StringVar(&configDir, "config-dir", "",
		"directory for VolSync config files (default is $HOME/.volsync)")
}

func setupConfigDir() {
	if configDir == "" {
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)
		configDir = path.Join(home, ".volsync")
		err = os.MkdirAll(configDir, 0755)
		cobra.CheckErr(err)
	}
}
