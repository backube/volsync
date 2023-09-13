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

package rsynctls

import (
	"regexp"
)

var rsyncTLSRegex = regexp.MustCompile(
	`([sS]ent)\s.+([bB]ytes)\s.+([rR]eceived)\s.+([bB]ytes)|` +
		`([tT]otal size)|` +
		`([rR]sync completed in)`)

var rsyncTLSRegexFailures = regexp.MustCompile(
	`^\s*([rR]sync)|` +
		`^\s*(disk[rR]sync)|` +
		`([fF]ail)|` +
		`([eE]rror)`)

// Filter rsync log lines for a successful move job
func LogLineFilterSuccess(line string) *string {
	if rsyncTLSRegex.MatchString(line) {
		return &line
	}
	return nil
}

func LogLineFilterFailure(line string) *string {
	// Match first against the same stuff we do for success
	if rsyncTLSRegex.MatchString(line) {
		return &line
	}

	// Also match some specific failure lines
	if rsyncTLSRegexFailures.MatchString(line) {
		return &line
	}

	return nil
}
