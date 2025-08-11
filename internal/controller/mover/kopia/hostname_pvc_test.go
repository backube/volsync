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

	"k8s.io/utils/ptr"
)

func TestHostnameGenerationWithPVC(t *testing.T) {
	tests := getHostnameWithPVCTestCases()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateHostname(tt.hostname, tt.pvcName, tt.namespace, tt.objectName)
			if result != tt.expected {
				t.Errorf("generateHostname() = %q, expected %q\nDescription: %s", result, tt.expected, tt.description)
			}

			// Additional validation: ensure result contains only valid characters for generated hostnames
			// (custom hostnames are passed through as-is without validation)
			validateHostnameCharacters(t, tt.hostname, result)

			// Ensure generated hostnames don't start or end with separators (unless it's the fallback)
			// (custom hostnames are passed through as-is)
			validateHostnameTrimming(t, tt.hostname, result)
		})
	}
}

type hostnameWithPVCTestCase struct {
	name        string
	hostname    *string
	pvcName     *string
	namespace   string
	objectName  string
	expected    string
	description string
}

// getHostnameWithPVCTestCases is defined in hostname_pvc_split.go
// getCustomHostnameCases is defined in hostname_pvc_split.go
// getNamespaceWithPVCCases is defined in hostname_pvc_split.go
// getEdgeAndFallbackCases is defined in hostname_pvc_split.go

// validateHostnameCharacters validates that generated hostnames contain only valid characters
func validateHostnameCharacters(t *testing.T, hostname *string, result string) {
	if hostname == nil && result != defaultUsername {
		for _, r := range result {
			validChar := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-'
			if !validChar {
				t.Errorf("generateHostname() returned invalid character %c in result %q", r, result)
			}
		}
	}
}

// validateHostnameTrimming validates that generated hostnames don't have leading/trailing separators
func validateHostnameTrimming(t *testing.T, hostname *string, result string) {
	if hostname == nil && result != defaultUsername {
		hasBadPrefix := strings.HasPrefix(result, "-") || strings.HasPrefix(result, ".")
		hasBadSuffix := strings.HasSuffix(result, "-") || strings.HasSuffix(result, ".")
		if hasBadPrefix || hasBadSuffix {
			t.Errorf("generateHostname() returned result %q with leading/trailing separators", result)
		}
	}
}

func TestUsernameGenerationWithNamespace(t *testing.T) {
	tests := getUsernameWithNamespaceTestCases()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateUsername(tt.username, tt.objectName, tt.namespace)
			if result != tt.expected {
				t.Errorf("generateUsername() = %q, expected %q\nDescription: %s", result, tt.expected, tt.description)
			}
		})
	}
}

type usernameWithNamespaceTestCase struct {
	name        string
	username    *string
	objectName  string
	namespace   string
	expected    string
	description string
}

// getUsernameWithNamespaceTestCases returns test cases for username with namespace generation
func getUsernameWithNamespaceTestCases() []usernameWithNamespaceTestCase {
	var cases []usernameWithNamespaceTestCase
	cases = append(cases, getCustomUsernameCases()...)
	cases = append(cases, getNamespaceAppendingCases()...)
	cases = append(cases, getUsernameEdgeCases()...)
	return cases
}

// getCustomUsernameCases tests custom username priority
func getCustomUsernameCases() []usernameWithNamespaceTestCase {
	return []usernameWithNamespaceTestCase{
		{
			name:        "custom username ignores namespace",
			username:    ptr.To("custom-user"),
			objectName:  "app-backup",
			namespace:   "production",
			expected:    "custom-user",
			description: "Should return custom username when provided, ignoring namespace",
		},
	}
}

// getNamespaceAppendingCases tests namespace appending logic
func getNamespaceAppendingCases() []usernameWithNamespaceTestCase {
	return []usernameWithNamespaceTestCase{
		{
			name:        "short object name with namespace",
			username:    nil,
			objectName:  "app",
			namespace:   "prod",
			expected:    "app-prod",
			description: "Should append namespace when combined length is short",
		},
		{
			name:        "object name with short namespace",
			username:    nil,
			objectName:  "application-backup",
			namespace:   "ns",
			expected:    "application-backup-ns",
			description: "Should append short namespace to longer object name",
		},
		{
			name:        "very long object name prevents namespace",
			username:    nil,
			objectName:  strings.Repeat("a", 45),
			namespace:   "production",
			expected:    strings.Repeat("a", 45),
			description: "Should not append namespace when object name is very long",
		},
		{
			name:        "object name at limit prevents namespace",
			username:    nil,
			objectName:  strings.Repeat("a", 50),
			namespace:   "prod",
			expected:    strings.Repeat("a", 50),
			description: "Should not append namespace when object name is at limit",
		},
		{
			name:        "empty namespace",
			username:    nil,
			objectName:  "app-backup",
			namespace:   "",
			expected:    "app-backup",
			description: "Should not append empty namespace",
		},
		{
			name:        "namespace with special characters",
			username:    nil,
			objectName:  "app-backup",
			namespace:   "prod@env",
			expected:    "app-backup-prodenv",
			description: "Should sanitize namespace before appending",
		},
		{
			name:        "namespace with underscores",
			username:    nil,
			objectName:  "app-backup",
			namespace:   "prod_env",
			expected:    "app-backup-prod_env",
			description: "Should preserve underscores in namespace",
		},
	}
}

// getUsernameEdgeCases tests edge cases
func getUsernameEdgeCases() []usernameWithNamespaceTestCase {
	return []usernameWithNamespaceTestCase{
		{
			name:        "namespace becomes empty after sanitization",
			username:    nil,
			objectName:  "app-backup",
			namespace:   "@#$%^&*()",
			expected:    "app-backup",
			description: "Should not append namespace that becomes empty after sanitization",
		},
		{
			name:        "object name with namespace both need sanitization",
			username:    nil,
			objectName:  "app@backup",
			namespace:   "prod@env",
			expected:    "appbackup-prodenv",
			description: "Should sanitize both object name and namespace",
		},
		{
			name:        "edge case at exactly 50 chars with separator",
			username:    nil,
			objectName:  strings.Repeat("a", 23),
			namespace:   strings.Repeat("b", 26),
			expected:    strings.Repeat("a", 23) + "-" + strings.Repeat("b", 26),
			description: "Should allow exactly 50 chars including separator",
		},
		{
			name:        "edge case just under limit",
			username:    nil,
			objectName:  strings.Repeat("a", 22),
			namespace:   strings.Repeat("b", 26),
			expected:    strings.Repeat("a", 22) + "-" + strings.Repeat("b", 26),
			description: "Should allow just under 50 chars",
		},
		{
			name:        "edge case exceeding limit",
			username:    nil,
			objectName:  strings.Repeat("a", 24),
			namespace:   strings.Repeat("b", 26),
			expected:    strings.Repeat("a", 24),
			description: "Should not append namespace when combined exceeds 50 chars",
		},
	}
}

func TestSanitizeForHostname(t *testing.T) {
	tests := getSanitizeTestCases()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeForHostname(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeForHostname(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

type sanitizeTestCase struct {
	name     string
	input    string
	expected string
}

func getSanitizeTestCases() []sanitizeTestCase {
	return []sanitizeTestCase{
		{
			name:     "no changes needed",
			input:    "valid-hostname",
			expected: "valid-hostname",
		},
		{
			name:     "underscores to hyphens",
			input:    "host_name_test",
			expected: "host-name-test",
		},
		{
			name:     "special characters removed",
			input:    "host@name#test",
			expected: "hostnametest",
		},
		{
			name:     "dots preserved",
			input:    "host.name.test",
			expected: "host.name.test",
		},
		{
			name:     "mixed case preserved",
			input:    "HostName.Test",
			expected: "HostName.Test",
		},
		{
			name:     "leading and trailing trimmed",
			input:    "---hostname---",
			expected: "hostname",
		},
		{
			name:     "dots trimmed",
			input:    "...hostname...",
			expected: "hostname",
		},
		{
			name:     "numbers preserved",
			input:    "host123name456",
			expected: "host123name456",
		},
		{
			name:     "all invalid chars",
			input:    "@#$%^&*()",
			expected: "",
		},
		{
			name:     "unicode removed",
			input:    "host-ñame-test",
			expected: "host-ame-test",
		},
	}
}

// TestHostnameGenerationMultiTenancyScenarios tests real-world multi-tenancy use cases
func TestHostnameGenerationMultiTenancyScenarios(t *testing.T) {
	testCases := []struct {
		name        string
		pvcName     string
		namespace   string
		objectName  string
		expected    string
		description string
	}{
		{
			name:        "production app data",
			pvcName:     "app-data",
			namespace:   "production",
			objectName:  "backup-job",
			expected:    "production",
			description: "Production environment uses namespace only",
		},
		{
			name:        "development with long names",
			pvcName:     "development-application-persistent-storage-volume",
			namespace:   "development",
			objectName:  "backup-job",
			expected:    "development",
			description: "Development environment uses namespace only",
		},
		{
			name:        "staging environment",
			pvcName:     "staging-db",
			namespace:   "staging",
			objectName:  "backup-job",
			expected:    "staging",
			description: "Staging environment uses namespace only",
		},
		{
			name:        "tenant isolation",
			pvcName:     "tenant-data",
			namespace:   "tenant-customer-a",
			objectName:  "backup-job",
			expected:    "tenant-customer-a",
			description: "Multi-tenant scenario with clear namespace isolation",
		},
		{
			name:        "very long tenant name",
			pvcName:     "data",
			namespace:   "very-long-tenant-namespace-that-exceeds-reasonable-length",
			objectName:  "backup-job",
			expected:    "very-long-tenant-namespace-that-exceeds-reasonable-length",
			description: "Long tenant names are fully preserved",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := generateHostname(nil, &tc.pvcName, tc.namespace, tc.objectName)
			if result != tc.expected {
				t.Errorf("Multi-tenancy test %s failed: got %q, expected %q\nDescription: %s",
					tc.name, result, tc.expected, tc.description)
			}
		})
	}
}

// TestHostnameGenerationNamespaceOnly validates namespace-only hostname generation
func TestHostnameGenerationNamespaceOnly(t *testing.T) {
	tests := map[string]struct {
		namespace  string
		pvcName    string
		objectName string
		expected   string
	}{
		"test-ns with app-backup PVC": {
			namespace:  "test-ns",
			pvcName:    "app-backup",
			objectName: "replication-src",
			expected:   "test-ns",
		},
		"prod with database PVC": {
			namespace:  "prod",
			pvcName:    "database",
			objectName: "replication-src",
			expected:   "prod",
		},
		"my-namespace with service_name PVC": {
			namespace:  "my-namespace",
			pvcName:    "service_name",
			objectName: "replication-src",
			expected:   "my-namespace",
		},
		"ns with app PVC": {
			namespace:  "ns",
			pvcName:    "app",
			objectName: "replication-src",
			expected:   "ns",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := generateHostname(nil, &tc.pvcName, tc.namespace, tc.objectName)
			if result != tc.expected {
				t.Errorf("Expected %s, got %s", tc.expected, result)
			}
		})
	}
}
