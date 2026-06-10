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
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
)

func addCLIRsyncTLSMoverSecurityContextFlags(cmdToUpdate *cobra.Command, isReplicationDestination bool) {
	crName := "ReplicationSource"
	if isReplicationDestination {
		crName = "ReplicationDestination"
	}
	cmdToUpdate.Flags().String("runasgroup", "",
		fmt.Sprintf("MoverSecurityContext runAsGroup to use in the %s (only if rsynctls=true)", crName))
	cmdToUpdate.Flags().String("runasuser", "",
		fmt.Sprintf("MoverSecurityContext runAsUser to use in the %s (only if rsynctls=true)", crName))
	cmdToUpdate.Flags().String("fsgroup", "",
		fmt.Sprintf("MoverSecurityContext fsGroup to use in the %s (only if rsynctls=true)", crName))
	// set runAsNonRoot as a string value with "" as default, as we don't want to
	// specify moverSecurityContext.runAsNonRoot unless the user sets this flag
	cmdToUpdate.Flags().String("runasnonroot", "",
		fmt.Sprintf("MoverSecurityContext runAsNonRoot (true/false) setting to use in the %s (only if rsynctls=true)",
			crName))
	cmdToUpdate.Flags().String("seccompprofiletype", "",
		fmt.Sprintf("MoverSecurityContext SeccompProfile.Type to use in the %s (only if rsynctls=true)", crName))
}

//nolint:funlen
func parseCLIRsyncTLSMoverSecurityContextFlags(cmd *cobra.Command) (*corev1.PodSecurityContext, error) {
	moverSecurityContext := &corev1.PodSecurityContext{}
	moverSecurityContextUpdated := false

	runAsGroupStr, err := cmd.Flags().GetString("runasgroup")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch runasgroup, %w", err)
	}
	if runAsGroupStr != "" {
		runAsGroupInt64, err := strconv.ParseInt(runAsGroupStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse runasgroup, %w", err)
		}
		moverSecurityContext.RunAsGroup = &runAsGroupInt64
		moverSecurityContextUpdated = true
	}

	runAsUserStr, err := cmd.Flags().GetString("runasuser")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch runasuser, %w", err)
	}
	if runAsUserStr != "" {
		runAsUserInt64, err := strconv.ParseInt(runAsUserStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse runasuser, %w", err)
		}
		moverSecurityContext.RunAsUser = &runAsUserInt64
		moverSecurityContextUpdated = true
	}

	fsGroupStr, err := cmd.Flags().GetString("fsgroup")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch fsgroup, %w", err)
	}
	if fsGroupStr != "" {
		fsGroupInt64, err := strconv.ParseInt(fsGroupStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse fsgroup, %w", err)
		}
		moverSecurityContext.FSGroup = &fsGroupInt64
		moverSecurityContextUpdated = true
	}

	runAsNonRootStr, err := cmd.Flags().GetString("runasnonroot")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch runasnonroot, %w", err)
	}
	if runAsNonRootStr != "" {
		runAsNonRootBool, err := strconv.ParseBool(runAsNonRootStr)
		if err != nil {
			return nil, fmt.Errorf("Failed to parse runasnonroot, %w", err)
		}
		moverSecurityContext.RunAsNonRoot = &runAsNonRootBool
		moverSecurityContextUpdated = true
	}

	secCompProfileTypeStr, err := cmd.Flags().GetString("seccompprofiletype")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch seccompprofiletype, %w", err)
	}
	if secCompProfileTypeStr != "" {
		if corev1.SeccompProfileType(secCompProfileTypeStr) != corev1.SeccompProfileTypeLocalhost &&
			corev1.SeccompProfileType(secCompProfileTypeStr) != corev1.SeccompProfileTypeRuntimeDefault &&
			corev1.SeccompProfileType(secCompProfileTypeStr) != corev1.SeccompProfileTypeUnconfined {
			return nil, fmt.Errorf("unsupported seccompprofiletype: %v", secCompProfileTypeStr)
		}
		moverSecurityContext.SeccompProfile = &corev1.SeccompProfile{
			Type: corev1.SeccompProfileType(secCompProfileTypeStr),
		}
		moverSecurityContextUpdated = true
	}

	if moverSecurityContextUpdated {
		return moverSecurityContext, nil
	}

	// No need to set a moverSecurityContext
	return nil, nil
}
