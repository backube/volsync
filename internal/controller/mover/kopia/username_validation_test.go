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
	"strings"
	"testing"
)

func TestGenerateUsername(t *testing.T) {
	tests := getGenerateUsernameTestCases()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateUsername(tt.username, tt.objectName, "")
			if result != tt.expected {
				t.Errorf("generateUsername() = %q, expected %q\nDescription: %s", result, tt.expected, tt.description)
			}

			// Additional validation: ensure result contains only valid characters for generated usernames
			// (custom usernames are passed through as-is without validation)
			validateUsernameCharacters(t, tt.username, result)

			// Ensure generated usernames don't start or end with separators (unless it's the fallback)
			// (custom usernames are passed through as-is)
			validateUsernameTrimming(t, tt.username, result)
		})
	}
}

// usernameTestCase is defined in username_split.go

// getGenerateUsernameTestCases is defined in username_split.go and returns all test cases split into smaller functions

// validateUsernameCharacters validates that generated usernames contain only valid characters
func validateUsernameCharacters(t *testing.T, username *string, result string) {
	if username == nil && result != defaultUsername {
		for _, r := range result {
			validChar := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_'
			if !validChar {
				t.Errorf("generateUsername() returned invalid character %c in result %q", r, result)
			}
		}
	}
}

// validateUsernameTrimming validates that generated usernames don't have leading/trailing separators
func validateUsernameTrimming(t *testing.T, username *string, result string) {
	if username == nil && result != defaultUsername {
		hasBadPrefix := strings.HasPrefix(result, "-") || strings.HasPrefix(result, "_")
		hasBadSuffix := strings.HasSuffix(result, "-") || strings.HasSuffix(result, "_")
		if hasBadPrefix || hasBadSuffix {
			t.Errorf("generateUsername() returned result %q with leading/trailing separators", result)
		}
	}
}

func TestGenerateUsernameEdgeCases(t *testing.T) {
	// Test with very long names to ensure it's truncated to maxUsernameLength
	veryLongName := strings.Repeat("a-", 1000) + "backup"
	result := generateUsername(nil, veryLongName, "")
	if len(result) > maxUsernameLength {
		t.Errorf("Expected long name to be truncated to max length %d, got length %d", maxUsernameLength, len(result))
	}

	// Test with only invalid characters followed by valid ones
	mixedName := "!!!abc123"
	result = generateUsername(nil, mixedName, "")
	expected := "abc123"
	if result != expected {
		t.Errorf("Expected %q for mixed invalid/valid name, got %q", expected, result)
	}

	// Test with valid chars followed by invalid ones
	mixedName2 := "abc123!!!"
	result = generateUsername(nil, mixedName2, "")
	expected = "abc123"
	if result != expected {
		t.Errorf("Expected %q for valid/invalid mixed name, got %q", expected, result)
	}

	// Test with alternating valid/invalid characters
	alternatingName := "a!b@c#d$e%f^g"
	result = generateUsername(nil, alternatingName, "")
	expected = "abcdefg"
	if result != expected {
		t.Errorf("Expected %q for alternating chars, got %q", expected, result)
	}
}

func TestGenerateUsernameBehaviorPreservation(t *testing.T) {
	// Test that existing behavior is preserved for valid names
	// This ensures backward compatibility with existing deployments
	testCases := []struct {
		objectName string
		expected   string
	}{
		{"app-backup", "app-backup"},
		{"database_backup", "database_backup"},
		{"service123", "service123"},
		{"MyApp-Backup_123", "MyApp-Backup_123"},
		{"simple", "simple"},
		{"backup-service-v1", "backup-service-v1"},
	}

	for _, tc := range testCases {
		t.Run(tc.objectName, func(t *testing.T) {
			result := generateUsername(nil, tc.objectName, "")
			if result != tc.expected {
				t.Errorf("Behavior changed for %q: got %q, expected %q", tc.objectName, result, tc.expected)
			}
		})
	}
}
