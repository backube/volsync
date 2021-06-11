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

func generateForgetOptions(policy *scribev1alpha1.ResticRetainPolicy) string {
	defaultForget := "--keep-last 1"

	// Retain policy isn't present
	if policy == nil {
		return defaultForget
	}

	var forget string
	optionTable := []struct {
		opt   string
		value *int32
	}{
		{"--keep-hourly", policy.Hourly},
		{"--keep-daily", policy.Daily},
		{"--keep-weekly", policy.Weekly},
		{"--keep-monthly", policy.Monthly},
		{"--keep-yearly", policy.Yearly},
	}
	for _, v := range optionTable {
		if v.value != nil {
			forget += fmt.Sprintf(" %s %d", v.opt, *v.value)
		}
	}
	if policy.Within != nil {
		forget += fmt.Sprintf(" --keep-within %s", *policy.Within)
	}

	if len(forget) == 0 { // Retain policy was present, but empty
		return defaultForget
	}
	return forget
}
