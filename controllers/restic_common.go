/*
Copyright 2020 The Scribe authors.

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package controllers

import (
	"fmt"

	scribev1alpha1 "github.com/backube/scribe/api/v1alpha1"
)

func generateForgetOptions(inst scribev1alpha1.ReplicationSource) string {
	defaultForget := "--keep-last 1"

	// Retain portion of CR isn't present
	if inst.Spec.Restic.Retain == nil {
		return defaultForget
	}

	var forget string
	optionTable := []struct {
		opt   string
		value *int32
	}{
		{"--keep-hourly", inst.Spec.Restic.Retain.Hourly},
		{"--keep-daily", inst.Spec.Restic.Retain.Daily},
		{"--keep-weekly", inst.Spec.Restic.Retain.Weekly},
		{"--keep-monthly", inst.Spec.Restic.Retain.Monthly},
		{"--keep-yearly", inst.Spec.Restic.Retain.Yearly},
	}
	for _, v := range optionTable {
		if v.value != nil {
			forget += fmt.Sprintf(" %s %d", v.opt, *v.value)
		}
	}
	if inst.Spec.Restic.Retain.Within != nil {
		forget += fmt.Sprintf(" --keep-within %s", *inst.Spec.Restic.Retain.Within)
	}

	if len(forget) == 0 { // Retain portion was present but empty
		return defaultForget
	}
	return forget
}
