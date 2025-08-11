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

import "testing"

func TestSanitizeFilesystemPath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Normal cases
		{"simple path", "backups", "backups"},
		{"path with subdirectory", "backups/daily", "backups/daily"},
		{"multiple subdirectories", "backups/2024/01/15", "backups/2024/01/15"},
		
		// Security: Path traversal attempts
		{"parent directory reference", "../etc/passwd", "etc/passwd"},
		{"multiple parent references", "../../etc/passwd", "etc/passwd"},
		{"parent reference in middle", "backups/../etc/passwd", "backups/etc/passwd"},
		{"complex traversal", "../../../etc/../passwd", "etc/passwd"},
		
		// Leading/trailing slashes
		{"leading slash", "/backups", "backups"},
		{"trailing slash", "backups/", "backups"},
		{"both slashes", "/backups/", "backups"},
		
		// Current directory references
		{"current directory", "./backups", "backups"},
		{"current directory in middle", "backups/./daily", "backups/daily"},
		
		// Empty and edge cases
		{"empty string", "", "backups"},
		{"only dots", "..", "backups"},
		{"only slash", "/", "backups"},
		{"dots and slashes", "/../", "backups"},
		{"complex empty", "/.././../", "backups"},
		
		// Mixed cases
		{"mixed traversal", "backups/../daily/../../etc", "backups/daily/etc"},
		{"spaces in path", "my backups/daily", "my backups/daily"},
		{"special chars", "backups-2024_01", "backups-2024_01"},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeFilesystemPath(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeFilesystemPath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSanitizeFilesystemPath_SecurityCases(t *testing.T) {
	// Additional focused security tests
	securityTests := []struct {
		name        string
		input       string
		shouldBlock string // what we're trying to prevent access to
	}{
		{"prevent /etc access", "../../../etc/passwd", "/etc/passwd"},
		{"prevent root access", "../../../", "/"},
		{"prevent home access", "../../home/user/.ssh/id_rsa", "/home/user/.ssh/id_rsa"},
		{"complex traversal", "backups/../../../../../../etc/shadow", "/etc/shadow"},
		{"url encoded traversal", "..%2F..%2Fetc", "..%2F..%2Fetc"}, // should be treated literally, but .. at start gets removed
		{"null byte injection", "backups\x00/etc/passwd", "backups\x00/etc/passwd"}, // null byte preserved
	}
	
	for _, tt := range securityTests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeFilesystemPath(tt.input)
			// Ensure the result doesn't start with ".." and doesn't contain "/.."
			if result[0:1] == "/" {
				t.Errorf("sanitizeFilesystemPath(%q) = %q, starts with /", tt.input, result)
			}
			if len(result) >= 2 && result[0:2] == ".." {
				t.Errorf("sanitizeFilesystemPath(%q) = %q, starts with ..", tt.input, result)
			}
			// Check result doesn't contain any parent directory references
			if contains(result, "../") || contains(result, "/..") {
				t.Errorf("sanitizeFilesystemPath(%q) = %q, contains parent directory reference", tt.input, result)
			}
		})
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}