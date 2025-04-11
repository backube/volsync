//go:build !disable_rclone

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

package rclone

import (
	"regexp"
)

var rcloneRegex = regexp.MustCompile(
	`^\s*([tT]ransferred:)|` +
		`^\s*([cC]hecks:)|` +
		`^\s*([dD]eleted:)|` +
		`^\s*([eE]lapsed time:)|` +
		`^\s*(Rclone completed in)`)

// Filter rclone log lines for a successful mover job
func LogLineFilterSuccess(line string) *string {
	if rcloneRegex.MatchString(line) {
		return &line
	}
	return nil
}
