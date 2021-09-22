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
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
)

// volsyncVersion value is set at build time via ldflags
var volsyncVersion = "0.0.0"

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: i18n.T("Print the version of the VolSync CLI"),
	Long: templates.LongDesc(i18n.T(`
	Display the version number of the VolSync CLI.
	`)),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Version: %s\n", volsyncVersion)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
