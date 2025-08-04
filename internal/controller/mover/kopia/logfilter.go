//go:build !disable_kopia

/*
Copyright 2024 The VolSync authors.

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

package kopia

import (
	"regexp"
)

var kopiaRegex = regexp.MustCompile(
	`^\s*([sS]napshot)\s.+([cC]reated)|` +
		`^\s*([uU]ploaded)\s.+([bB]ytes)|` +
		`^\s*([rR]estored)\s.+([fF]iles)|` +
		`^\s*([sS]uccessfully)|` +
		`^\s*([cC]onnected to repository)|` +
		`^\s*([rR]epository)\s.+([oO]pened)|` +
		`^\s*([mM]aintenance)|` +
		`^\s*([cC]ompression)|` +
		`^\s*([nN]o changes)|` +
		`^\s*([sS]kipping)|` +
		`(KOPIA_OPTIONS)|` +
		`([iI]nitialize [rR]epository)|` +
		`^\s*([fF]atal)|` +
		`^\s*(ERROR)|` +
		`^\s*([kK]opia completed in)`)

// Filter kopia log lines for a successful move job
func LogLineFilterSuccess(line string) *string {
	if kopiaRegex.MatchString(line) {
		return &line
	}
	return nil
}