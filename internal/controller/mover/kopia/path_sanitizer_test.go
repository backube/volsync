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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("sanitizeFilesystemPath", func() {
	DescribeTable("should sanitize paths correctly",
		func(input, expected string) {
			result := sanitizeFilesystemPath(input)
			Expect(result).To(Equal(expected))
		},
		// Normal cases
		Entry("simple path", "backups", "backups"),
		Entry("path with subdirectory", "backups/daily", "backups/daily"),
		Entry("multiple subdirectories", "backups/2024/01/15", "backups/2024/01/15"),
		
		// Path traversal attempts
		Entry("parent directory reference", "../etc/passwd", "etc/passwd"),
		Entry("multiple parent references", "../../etc/passwd", "etc/passwd"),
		Entry("parent reference in middle", "backups/../etc/passwd", "backups/etc/passwd"),
		Entry("complex traversal", "../../../etc/../passwd", "etc/passwd"),
		
		// Leading/trailing slashes
		Entry("leading slash", "/backups", "backups"),
		Entry("trailing slash", "backups/", "backups"),
		Entry("both slashes", "/backups/", "backups"),
		
		// Current directory references
		Entry("current directory", "./backups", "backups"),
		Entry("current directory in middle", "backups/./daily", "backups/daily"),
		
		// Empty and edge cases
		Entry("empty string", "", "backups"),
		Entry("only dots", "..", "backups"),
		Entry("only slash", "/", "backups"),
		Entry("dots and slashes", "/../", "backups"),
		Entry("complex empty", "/.././../", "backups"),
		
		// Mixed cases
		Entry("mixed traversal", "backups/../daily/../../etc", "backups/daily/etc"),
		Entry("windows-style path", "backups\\daily", "backups\\daily"), // Backslash treated as normal char
		Entry("spaces in path", "my backups/daily", "my backups/daily"),
		Entry("special chars", "backups-2024_01", "backups-2024_01"),
	)

	It("should handle very long paths", func() {
		// Create a very long but valid path
		longPath := "backups"
		for i := 0; i < 20; i++ {
			longPath += "/level" + string(rune('0'+i))
		}
		
		result := sanitizeFilesystemPath(longPath)
		Expect(result).To(Equal(longPath)) // Should preserve the valid long path
	})

	It("should handle path traversal at different positions", func() {
		testCases := []struct {
			input    string
			expected string
		}{
			{"../backups", "backups"},
			{"backups/..", "backups"},
			{"backups/../daily", "backups/daily"},
			{"../../../backups", "backups"},
			{"backups/../../../etc", "backups/etc"},
		}
		
		for _, tc := range testCases {
			result := sanitizeFilesystemPath(tc.input)
			Expect(result).To(Equal(tc.expected), "Failed for input: %s", tc.input)
		}
	})

	It("should preserve valid special characters", func() {
		validPaths := []string{
			"backups_2024",
			"backups-daily",
			"backups.tar",
			"backups 2024",
			"backups(1)",
			"backups[daily]",
		}
		
		for _, path := range validPaths {
			result := sanitizeFilesystemPath(path)
			Expect(result).To(Equal(path), "Should preserve: %s", path)
		}
	})
})