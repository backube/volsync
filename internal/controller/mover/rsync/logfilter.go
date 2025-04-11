//go:build !disable_rsync

/*
Copyright 2022 The VolSync authors.

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

package rsync

import (
	"regexp"
)

var rsyncRegex = regexp.MustCompile(
	`^\s*([sS]ent)\s.+([bB]ytes)\s.+([rR]eceived)\s.+([bB]ytes)|` +
		`^\s*([tT]otal size)|` +
		`^\s*([rR]sync completed in)`)

// Filter rsync log lines for a successful move job
func LogLineFilterSuccess(line string) *string {
	if rsyncRegex.MatchString(line) {
		return &line
	}
	return nil
}
