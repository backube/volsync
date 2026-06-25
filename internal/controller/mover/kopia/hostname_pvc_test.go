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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"
)

type hostnameWithPVCTestCase struct {
	name        string
	hostname    *string
	pvcName     *string
	namespace   string
	objectName  string
	expected    string
	description string
}

// validateHostnameCharacters validates that generated hostnames contain only valid characters
func validateHostnameCharacters(hostname *string, result string) bool {
	if hostname == nil && result != defaultUsername {
		for _, r := range result {
			validChar := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-'
			if !validChar {
				return false
			}
		}
	}
	return true
}

// validateHostnameTrimming validates that generated hostnames don't have leading/trailing separators
func validateHostnameTrimming(hostname *string, result string) bool {
	if hostname == nil && result != defaultUsername {
		hasBadPrefix := strings.HasPrefix(result, "-") || strings.HasPrefix(result, ".")
		hasBadSuffix := strings.HasSuffix(result, "-") || strings.HasSuffix(result, ".")
		if hasBadPrefix || hasBadSuffix {
			return false
		}
	}
	return true
}

var _ = Describe("Hostname Generation With PVC", func() {
	DescribeTable("generates correct hostname",
		func(tt hostnameWithPVCTestCase) {
			result := generateHostname(tt.hostname, tt.pvcName, tt.namespace, tt.objectName)
			Expect(result).To(Equal(tt.expected), tt.description)

			// Additional validation: ensure result contains only valid characters for generated hostnames
			// (custom hostnames are passed through as-is without validation)
			Expect(validateHostnameCharacters(tt.hostname, result)).To(BeTrue(),
				"generateHostname() returned invalid characters in result %q", result)

			// Ensure generated hostnames don't start or end with separators (unless it's the fallback)
			// (custom hostnames are passed through as-is)
			Expect(validateHostnameTrimming(tt.hostname, result)).To(BeTrue(),
				"generateHostname() returned result %q with leading/trailing separators", result)
		},
		// Custom hostname cases
		Entry("custom hostname provided", hostnameWithPVCTestCase{
			hostname:    ptr.To("custom-host"),
			pvcName:     ptr.To("my-pvc"),
			namespace:   "test-ns",
			objectName:  "test-obj",
			expected:    "custom-host",
			description: "Should return custom hostname when provided, ignoring namespace and PVC",
		}),
		// Namespace with PVC cases
		Entry("namespace with PVC - ignore PVC", hostnameWithPVCTestCase{
			hostname:    nil,
			pvcName:     ptr.To("app-data"),
			namespace:   "production",
			objectName:  "test-obj",
			expected:    "production",
			description: "Should only use namespace, ignoring PVC name",
		}),
		Entry("namespace with PVC - sanitization", hostnameWithPVCTestCase{
			hostname:    nil,
			pvcName:     ptr.To("data_pvc_volume"),
			namespace:   "test_ns",
			objectName:  "test-obj",
			expected:    "test-ns",
			description: "Should sanitize namespace and ignore PVC name",
		}),
		Entry("namespace with PVC - special characters", hostnameWithPVCTestCase{
			hostname:    nil,
			pvcName:     ptr.To("data@pvc.storage"),
			namespace:   "prod@env",
			objectName:  "test-obj",
			expected:    "prodenv",
			description: "Should remove special characters from namespace and ignore PVC",
		}),
		Entry("namespace with PVC - long PVC ignored", hostnameWithPVCTestCase{
			hostname:    nil,
			pvcName:     ptr.To(strings.Repeat("very-long-pvc-name", 3)),
			namespace:   "my-namespace",
			objectName:  "test-obj",
			expected:    "my-namespace",
			description: "Should use only namespace, ignoring long PVC name",
		}),
		Entry("namespace only - no PVC", hostnameWithPVCTestCase{
			hostname:    nil,
			pvcName:     nil,
			namespace:   "test-namespace",
			objectName:  "test-obj",
			expected:    "test-namespace",
			description: "Should use namespace when PVC is nil",
		}),
		Entry("namespace only - empty PVC", hostnameWithPVCTestCase{
			hostname:    nil,
			pvcName:     ptr.To(""),
			namespace:   "test-namespace",
			objectName:  "test-obj",
			expected:    "test-namespace",
			description: "Should use namespace when PVC is empty string",
		}),
		// Sanitization edge cases
		Entry("namespace with leading and trailing special chars", hostnameWithPVCTestCase{
			hostname:    nil,
			pvcName:     ptr.To("-_data-pvc_-"),
			namespace:   "-_test-ns_-",
			objectName:  "test-obj",
			expected:    "test-ns",
			description: "Should trim leading and trailing hyphens from namespace and ignore PVC",
		}),
		Entry("PVC becomes empty after sanitization", hostnameWithPVCTestCase{
			hostname:    nil,
			pvcName:     ptr.To("@#$%^&*()"),
			namespace:   "test-ns",
			objectName:  "test-obj",
			expected:    "test-ns",
			description: "Should use only namespace, ignoring invalid PVC",
		}),
		Entry("namespace becomes empty - fallback to namespace-name", hostnameWithPVCTestCase{
			hostname:    nil,
			pvcName:     ptr.To("data-pvc"),
			namespace:   "@#$%^&*()",
			objectName:  "test-obj",
			expected:    "test-obj",
			description: "Should fallback to namespace-name format when namespace is invalid",
		}),
		// Length edge cases
		Entry("long namespace only", hostnameWithPVCTestCase{
			hostname:    nil,
			pvcName:     nil,
			namespace:   strings.Repeat("long-namespace-", 5),
			objectName:  "test-obj",
			expected:    strings.Repeat("long-namespace-", 5)[:len(strings.Repeat("long-namespace-", 5))-1],
			description: "Should handle long namespace correctly",
		}),
		Entry("namespace with mixed case", hostnameWithPVCTestCase{
			hostname:    nil,
			pvcName:     ptr.To("MyApp-Data-PVC"),
			namespace:   "MyNamespace",
			objectName:  "test-obj",
			expected:    "MyNamespace",
			description: "Should preserve case in namespace and ignore PVC",
		}),
		Entry("namespace with numbers", hostnameWithPVCTestCase{
			hostname:    nil,
			pvcName:     ptr.To("pvc-123"),
			namespace:   "namespace-456",
			objectName:  "test-obj",
			expected:    "namespace-456",
			description: "Should handle numbers in namespace and ignore PVC",
		}),
		Entry("short namespace with PVC ignored", hostnameWithPVCTestCase{
			hostname:    nil,
			pvcName:     ptr.To("very-long-persistent-volume-claim-name-that-is-really-long"),
			namespace:   "ns",
			objectName:  "test-obj",
			expected:    "ns",
			description: "Should use namespace even when very short, ignoring PVC",
		}),
		Entry("namespace with PVC ignored", hostnameWithPVCTestCase{
			hostname:    nil,
			pvcName:     ptr.To("data"),
			namespace:   strings.Repeat("a", 45),
			objectName:  "test-obj",
			expected:    strings.Repeat("a", 45),
			description: "Should use namespace only, ignoring PVC",
		}),
		Entry("namespace with long PVC ignored", hostnameWithPVCTestCase{
			hostname:    nil,
			pvcName:     ptr.To("data1"),
			namespace:   strings.Repeat("a", 45),
			objectName:  "test-obj",
			expected:    strings.Repeat("a", 45),
			description: "Should use namespace only, ignoring PVC",
		}),
		// Dot handling cases
		Entry("PVC with dots ignored", hostnameWithPVCTestCase{
			hostname:    nil,
			pvcName:     ptr.To("my.app.pvc"),
			namespace:   "production",
			objectName:  "test-obj",
			expected:    "production",
			description: "Should use namespace only, ignoring PVC with dots",
		}),
		Entry("namespace with dots", hostnameWithPVCTestCase{
			hostname:    nil,
			pvcName:     ptr.To("data-pvc"),
			namespace:   "prod.region.cluster",
			objectName:  "test-obj",
			expected:    "prod.region.cluster",
			description: "Should preserve dots in namespace and ignore PVC",
		}),
		// Fallback cases
		Entry("trailing dots and hyphens", hostnameWithPVCTestCase{
			hostname:    nil,
			pvcName:     ptr.To("data-pvc..."),
			namespace:   "test-ns---",
			objectName:  "test-obj",
			expected:    "test-ns",
			description: "Should trim trailing dots and hyphens from namespace, ignore PVC",
		}),
		Entry("both namespace and PVC invalid - use object name", hostnameWithPVCTestCase{
			hostname:    nil,
			pvcName:     ptr.To("@#$%^&*()"),
			namespace:   "@#$%^&*()",
			objectName:  "test-obj",
			expected:    "test-obj",
			description: "Should use sanitized object name when namespace is invalid",
		}),
		Entry("everything invalid - use default", hostnameWithPVCTestCase{
			hostname:    nil,
			pvcName:     ptr.To("@#$%^&*()"),
			namespace:   "@#$%^&*()",
			objectName:  "@#$%^&*()",
			expected:    defaultUsername,
			description: "Should use volsync-default when everything is invalid",
		}),
	)
})

type pvcUsernameTestCase struct {
	username    *string
	objectName  string
	namespace   string
	expected    string
	description string
}

var _ = Describe("Username Generation Simplified", func() {
	DescribeTable("generates correct username",
		func(tt pvcUsernameTestCase) {
			result := generateUsername(tt.username, tt.objectName, tt.namespace)
			Expect(result).To(Equal(tt.expected), tt.description)
		},
		// Custom username priority
		Entry("custom username ignores namespace", pvcUsernameTestCase{
			username:    ptr.To("custom-user"),
			objectName:  "app-backup",
			namespace:   "production",
			expected:    "custom-user",
			description: "Should return custom username when provided, ignoring namespace",
		}),
		// Object name only logic (no namespace appending)
		Entry("short object name (no namespace)", pvcUsernameTestCase{
			username:    nil,
			objectName:  "app",
			namespace:   "prod",
			expected:    "app",
			description: "Should use object name only (no namespace appending)",
		}),
		Entry("object name with short namespace", pvcUsernameTestCase{
			username:    nil,
			objectName:  "application-backup",
			namespace:   "ns",
			expected:    "application-backup",
			description: "Should use object name only (no namespace appending)",
		}),
		Entry("very long object name (no namespace)", pvcUsernameTestCase{
			username:    nil,
			objectName:  strings.Repeat("a", 45),
			namespace:   "production",
			expected:    strings.Repeat("a", 45),
			description: "Should use object name only (no namespace appending)",
		}),
		Entry("object name at limit (no namespace)", pvcUsernameTestCase{
			username:    nil,
			objectName:  strings.Repeat("a", 50),
			namespace:   "prod",
			expected:    strings.Repeat("a", 50),
			description: "Should use object name only (no namespace appending)",
		}),
		Entry("empty namespace", pvcUsernameTestCase{
			username:    nil,
			objectName:  "app-backup",
			namespace:   "",
			expected:    "app-backup",
			description: "Should use object name only (empty namespace)",
		}),
		Entry("namespace with special characters", pvcUsernameTestCase{
			username:    nil,
			objectName:  "app-backup",
			namespace:   "prod@env",
			expected:    "app-backup",
			description: "Should use object name only (no namespace appending)",
		}),
		Entry("namespace with underscores", pvcUsernameTestCase{
			username:    nil,
			objectName:  "app-backup",
			namespace:   "prod_env",
			expected:    "app-backup",
			description: "Should use object name only (no namespace appending)",
		}),
		// Edge cases
		Entry("namespace becomes empty after sanitization", pvcUsernameTestCase{
			username:    nil,
			objectName:  "app-backup",
			namespace:   "@#$%^&*()",
			expected:    "app-backup",
			description: "Should use object name only (namespace ignored)",
		}),
		Entry("object name with namespace both need sanitization", pvcUsernameTestCase{
			username:    nil,
			objectName:  "app@backup",
			namespace:   "prod@env",
			expected:    "appbackup",
			description: "Should sanitize object name only (namespace ignored)",
		}),
		Entry("edge case at exactly 50 chars with separator", pvcUsernameTestCase{
			username:    nil,
			objectName:  strings.Repeat("a", 23),
			namespace:   strings.Repeat("b", 26),
			expected:    strings.Repeat("a", 23),
			description: "Should use object name only (namespace ignored)",
		}),
		Entry("edge case just under limit", pvcUsernameTestCase{
			username:    nil,
			objectName:  strings.Repeat("a", 22),
			namespace:   strings.Repeat("b", 26),
			expected:    strings.Repeat("a", 22),
			description: "Should use object name only (namespace ignored)",
		}),
		Entry("edge case exceeding limit", pvcUsernameTestCase{
			username:    nil,
			objectName:  strings.Repeat("a", 24),
			namespace:   strings.Repeat("b", 26),
			expected:    strings.Repeat("a", 24),
			description: "Should use object name only (namespace ignored)",
		}),
	)
})

type pvcSanitizeTestCase struct {
	input    string
	expected string
}

var _ = Describe("Sanitize For Hostname", func() {
	DescribeTable("sanitizes input correctly",
		func(tt pvcSanitizeTestCase) {
			result := sanitizeForHostname(tt.input)
			Expect(result).To(Equal(tt.expected))
		},
		Entry("no changes needed", pvcSanitizeTestCase{
			input:    "valid-hostname",
			expected: "valid-hostname",
		}),
		Entry("underscores to hyphens", pvcSanitizeTestCase{
			input:    "host_name_test",
			expected: "host-name-test",
		}),
		Entry("special characters removed", pvcSanitizeTestCase{
			input:    "host@name#test",
			expected: "hostnametest",
		}),
		Entry("dots preserved", pvcSanitizeTestCase{
			input:    "host.name.test",
			expected: "host.name.test",
		}),
		Entry("mixed case preserved", pvcSanitizeTestCase{
			input:    "HostName.Test",
			expected: "HostName.Test",
		}),
		Entry("leading and trailing trimmed", pvcSanitizeTestCase{
			input:    "---hostname---",
			expected: "hostname",
		}),
		Entry("dots trimmed", pvcSanitizeTestCase{
			input:    "...hostname...",
			expected: "hostname",
		}),
		Entry("numbers preserved", pvcSanitizeTestCase{
			input:    "host123name456",
			expected: "host123name456",
		}),
		Entry("all invalid chars", pvcSanitizeTestCase{
			input:    "@#$%^&*()",
			expected: "",
		}),
		Entry("unicode removed", pvcSanitizeTestCase{
			input:    "host-ñame-test",
			expected: "host-ame-test",
		}),
	)
})

type pvcMultiTenancyTestCase struct {
	pvcName     string
	namespace   string
	objectName  string
	expected    string
	description string
}

var _ = Describe("Hostname Generation Multi-Tenancy Scenarios", func() {
	DescribeTable("tests real-world multi-tenancy use cases",
		func(tc pvcMultiTenancyTestCase) {
			result := generateHostname(nil, &tc.pvcName, tc.namespace, tc.objectName)
			Expect(result).To(Equal(tc.expected), tc.description)
		},
		Entry("production app data", pvcMultiTenancyTestCase{
			pvcName:     "app-data",
			namespace:   "production",
			objectName:  "backup-job",
			expected:    "production",
			description: "Production environment uses namespace only",
		}),
		Entry("development with long names", pvcMultiTenancyTestCase{
			pvcName:     "development-application-persistent-storage-volume",
			namespace:   "development",
			objectName:  "backup-job",
			expected:    "development",
			description: "Development environment uses namespace only",
		}),
		Entry("staging environment", pvcMultiTenancyTestCase{
			pvcName:     "staging-db",
			namespace:   "staging",
			objectName:  "backup-job",
			expected:    "staging",
			description: "Staging environment uses namespace only",
		}),
		Entry("tenant isolation", pvcMultiTenancyTestCase{
			pvcName:     "tenant-data",
			namespace:   "tenant-customer-a",
			objectName:  "backup-job",
			expected:    "tenant-customer-a",
			description: "Multi-tenant scenario with clear namespace isolation",
		}),
		Entry("very long tenant name", pvcMultiTenancyTestCase{
			pvcName:     "data",
			namespace:   "very-long-tenant-namespace-that-exceeds-reasonable-length",
			objectName:  "backup-job",
			expected:    "very-long-tenant-namespace-that-exceeds-reasonable-length",
			description: "Long tenant names are fully preserved",
		}),
	)
})

type pvcNamespaceOnlyTestCase struct {
	namespace  string
	pvcName    string
	objectName string
	expected   string
}

var _ = Describe("Hostname Generation Namespace Only", func() {
	DescribeTable("validates namespace-only hostname generation",
		func(tc pvcNamespaceOnlyTestCase) {
			result := generateHostname(nil, &tc.pvcName, tc.namespace, tc.objectName)
			Expect(result).To(Equal(tc.expected))
		},
		Entry("test-ns with app-backup PVC", pvcNamespaceOnlyTestCase{
			namespace:  "test-ns",
			pvcName:    "app-backup",
			objectName: "replication-src",
			expected:   "test-ns",
		}),
		Entry("prod with database PVC", pvcNamespaceOnlyTestCase{
			namespace:  "prod",
			pvcName:    "database",
			objectName: "replication-src",
			expected:   "prod",
		}),
		Entry("my-namespace with service_name PVC", pvcNamespaceOnlyTestCase{
			namespace:  "my-namespace",
			pvcName:    "service_name",
			objectName: "replication-src",
			expected:   "my-namespace",
		}),
		Entry("ns with app PVC", pvcNamespaceOnlyTestCase{
			namespace:  "ns",
			pvcName:    "app",
			objectName: "replication-src",
			expected:   "ns",
		}),
	)
})
