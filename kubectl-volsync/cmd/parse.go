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
	"fmt"
	"strings"

	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

// Parse volume accessModes from a flag
func parseAccessModes(flagSet *pflag.FlagSet, flagName string) ([]corev1.PersistentVolumeAccessMode, error) {
	modes, err := flagSet.GetStringSlice(flagName)
	if err != nil {
		return nil, err
	}
	modeList := []corev1.PersistentVolumeAccessMode{}
	for _, m := range modes {
		if m != string(corev1.ReadOnlyMany) && m != string(corev1.ReadWriteMany) &&
			m != string(corev1.ReadWriteOnce) && m != string(corev1.ReadWriteOncePod) {
			return nil, fmt.Errorf("invalid access mode: %v", m)
		}
		modeList = append(modeList, corev1.PersistentVolumeAccessMode(m))
	}
	return modeList, nil
}

// Parse capacity (resource.Quatitiy) from a flag
func parseCapacity(flagSet *pflag.FlagSet, flagName string) (*resource.Quantity, error) {
	cstring, err := flagSet.GetString(flagName)
	if err != nil {
		return nil, err
	}
	if len(cstring) == 0 { // no option specified
		return nil, nil
	}
	capacity, err := resource.ParseQuantity(cstring)
	if err != nil {
		return nil, err
	}
	return &capacity, nil
}

// Parse CopyMethod from a flag
func parseCopyMethod(flagSet *pflag.FlagSet, flagName string,
	allowClone bool) (*volsyncv1alpha1.CopyMethodType, error) {
	allowedMethods := []volsyncv1alpha1.CopyMethodType{
		volsyncv1alpha1.CopyMethodDirect,
		volsyncv1alpha1.CopyMethodNone,
		volsyncv1alpha1.CopyMethodSnapshot,
	}
	if allowClone {
		allowedMethods = append(allowedMethods, volsyncv1alpha1.CopyMethodClone)
	}

	cm, err := flagSet.GetString(flagName)
	if err != nil {
		return nil, err
	}
	if cm == "" {
		return nil, nil
	}
	for _, m := range allowedMethods {
		if strings.EqualFold(cm, string(m)) {
			return &m, nil
		}
	}

	return nil, fmt.Errorf("invalid CopyMethod: %v", cm)
}
