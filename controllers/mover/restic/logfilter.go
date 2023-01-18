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

package restic

import (
	"regexp"
)

var resticRegex = regexp.MustCompile(
	`^\s*([pP]rocessed)\s.+([fF]iles)|` +
		`^\s*([sS]napshot)\s.+([sS]aved)|` +
		`^\s*([rR]epository)\s.+([oO]pened)|` +
		`^\s*([rR]estoring)|` +
		`^\s*([nN]o parent snapshot)|` +
		`^\s*([uU]sing parent snapshot)|` +
		`^\s*([aA]dded to the repository)|` +
		`^\s*(Restic completed in)`)

// Filter restic log lines for a successful move job
func LogLineFilterSuccess(line string) *string {
	if resticRegex.MatchString(line) {
		return &line
	}
	return nil
}
