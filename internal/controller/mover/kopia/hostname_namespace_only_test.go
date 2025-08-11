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
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"
)

// hostnameTestCase represents a test case for hostname generation
type hostnameTestCase struct {
	name        string
	hostname    *string
	pvcName     *string
	namespace   string
	objectName  string
	expected    string
	description string
}

// getBasicNamespaceTests returns test cases for basic namespace-only behavior
func getBasicNamespaceTests() []hostnameTestCase {
	return []hostnameTestCase{
		{
			name:        "simple namespace",
			hostname:    nil,
			pvcName:     nil,
			namespace:   "production",
			objectName:  "backup-job",
			expected:    "production",
			description: "Should use namespace only",
		},
		{
			name:        "namespace with PVC - PVC ignored",
			hostname:    nil,
			pvcName:     ptr.To("my-data-pvc"),
			namespace:   "production",
			objectName:  "backup-job",
			expected:    "production",
			description: "Should use namespace only, ignoring PVC",
		},
		{
			name:        "namespace with long PVC - PVC ignored",
			hostname:    nil,
			pvcName:     ptr.To("very-long-persistent-volume-claim-name-that-would-exceed-limits"),
			namespace:   "prod",
			objectName:  "backup-job",
			expected:    "prod",
			description: "Should use namespace only, ignoring long PVC",
		},
		{
			name:        "namespace with special characters in PVC",
			hostname:    nil,
			pvcName:     ptr.To("my_special@pvc.name"),
			namespace:   "staging",
			objectName:  "backup-job",
			expected:    "staging",
			description: "Should use namespace only, ignoring PVC with special chars",
		},
	}
}

// getNamespaceSanitizationTests returns test cases for namespace sanitization
func getNamespaceSanitizationTests() []hostnameTestCase {
	return []hostnameTestCase{
		{
			name:        "namespace with underscores",
			hostname:    nil,
			pvcName:     ptr.To("data-pvc"),
			namespace:   "test_namespace",
			objectName:  "backup-job",
			expected:    "test-namespace",
			description: "Should sanitize namespace (convert underscores) and ignore PVC",
		},
		{
			name:        "namespace with dots",
			hostname:    nil,
			pvcName:     ptr.To("data-pvc"),
			namespace:   "prod.region.cluster",
			objectName:  "backup-job",
			expected:    "prod.region.cluster",
			description: "Should preserve dots in namespace and ignore PVC",
		},
		{
			name:        "namespace with special characters",
			hostname:    nil,
			pvcName:     ptr.To("data-pvc"),
			namespace:   "test@namespace#123",
			objectName:  "backup-job",
			expected:    "testnamespace123",
			description: "Should sanitize namespace and ignore PVC",
		},
	}
}

// getMultiTenantTests returns test cases for multi-tenant scenarios
func getMultiTenantTests() []hostnameTestCase {
	return []hostnameTestCase{
		{
			name:        "tenant-a namespace",
			hostname:    nil,
			pvcName:     ptr.To("tenant-a-database"),
			namespace:   "tenant-a",
			objectName:  "backup-job",
			expected:    "tenant-a",
			description: "Multi-tenant: should use tenant namespace only",
		},
		{
			name:        "tenant-b namespace",
			hostname:    nil,
			pvcName:     ptr.To("tenant-b-database"),
			namespace:   "tenant-b",
			objectName:  "backup-job",
			expected:    "tenant-b",
			description: "Multi-tenant: should use tenant namespace only",
		},
		{
			name:        "customer namespace with app PVC",
			hostname:    nil,
			pvcName:     ptr.To("customer-app-data"),
			namespace:   "customer-prod",
			objectName:  "backup-job",
			expected:    "customer-prod",
			description: "Multi-tenant: should use customer namespace only",
		},
	}
}

// getEdgeCaseTests returns test cases for edge cases and fallbacks
func getEdgeCaseTests() []hostnameTestCase {
	tests := []hostnameTestCase{}
	tests = append(tests, getNamespaceLengthTests()...)
	tests = append(tests, getPVCEdgeCaseTests()...)
	tests = append(tests, getFallbackTests()...)
	return tests
}

func getNamespaceLengthTests() []hostnameTestCase {
	return []hostnameTestCase{
		{
			name:        "very long namespace",
			hostname:    nil,
			pvcName:     ptr.To("data"),
			namespace:   "very-long-namespace-name-that-is-still-valid-in-kubernetes",
			objectName:  "backup-job",
			expected:    "very-long-namespace-name-that-is-still-valid-in-kubernetes",
			description: "Should use full namespace even if long, ignore PVC",
		},
		{
			name:        "short namespace",
			hostname:    nil,
			pvcName:     ptr.To("very-long-pvc-name"),
			namespace:   "ns",
			objectName:  "backup-job",
			expected:    "ns",
			description: "Should use short namespace, ignore PVC",
		},
	}
}

func getPVCEdgeCaseTests() []hostnameTestCase {
	return []hostnameTestCase{
		{
			name:        "empty PVC name",
			hostname:    nil,
			pvcName:     ptr.To(""),
			namespace:   "namespace",
			objectName:  "backup-job",
			expected:    "namespace",
			description: "Should use namespace when PVC is empty",
		},
		{
			name:        "nil PVC",
			hostname:    nil,
			pvcName:     nil,
			namespace:   "namespace",
			objectName:  "backup-job",
			expected:    "namespace",
			description: "Should use namespace when PVC is nil",
		},
	}
}

func getFallbackTests() []hostnameTestCase {
	return []hostnameTestCase{
		{
			name:        "custom hostname overrides everything",
			hostname:    ptr.To("my-custom-hostname"),
			pvcName:     ptr.To("data-pvc"),
			namespace:   "production",
			objectName:  "backup-job",
			expected:    "my-custom-hostname",
			description: "Custom hostname should override namespace-only logic",
		},
		{
			name:        "invalid namespace falls back",
			hostname:    nil,
			pvcName:     ptr.To("data-pvc"),
			namespace:   "@#$%^&*()",
			objectName:  "backup-job",
			expected:    "backup-job",
			description: "Should fallback when namespace is invalid",
		},
		{
			name:        "empty namespace falls back",
			hostname:    nil,
			pvcName:     ptr.To("data-pvc"),
			namespace:   "",
			objectName:  "backup-job",
			expected:    "backup-job",
			description: "Should fallback when namespace is empty",
		},
	}
}

// TestHostnameIsAlwaysNamespaceOnly verifies that hostname generation
// ALWAYS uses only the namespace, never including PVC names
func TestHostnameIsAlwaysNamespaceOnly(t *testing.T) {
	// Combine all test cases
	var testCases []hostnameTestCase
	testCases = append(testCases, getBasicNamespaceTests()...)
	testCases = append(testCases, getNamespaceSanitizationTests()...)
	testCases = append(testCases, getMultiTenantTests()...)
	testCases = append(testCases, getEdgeCaseTests()...)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := generateHostname(tc.hostname, tc.pvcName, tc.namespace, tc.objectName)
			assert.Equal(t, tc.expected, result, tc.description)

			// Additional assertion: if namespace is valid and no custom hostname,
			// the result should NEVER contain the PVC name
			if tc.hostname == nil && tc.pvcName != nil && *tc.pvcName != "" && tc.namespace != "" {
				sanitizedNamespace := sanitizeForHostname(tc.namespace)
				if sanitizedNamespace != "" {
					// The result should be based on namespace, not containing PVC parts
					assert.NotContains(t, result, sanitizeForHostname(*tc.pvcName),
						"Hostname should not contain PVC name when namespace is valid")
				}
			}
		})
	}
}

// TestHostnameConsistencyAcrossSourceAndDestination verifies that
// the same namespace always generates the same hostname for both
// ReplicationSource and ReplicationDestination
func TestHostnameConsistencyAcrossSourceAndDestination(t *testing.T) {
	namespaces := []string{
		"production",
		"staging",
		"development",
		"tenant-a",
		"tenant-b",
		"customer-prod",
		"test-namespace",
		"my_namespace",
		"namespace.with.dots",
	}

	for _, ns := range namespaces {
		t.Run(ns, func(t *testing.T) {
			// Test with different PVC names to ensure they don't affect the result
			pvcNames := []*string{
				ptr.To("source-pvc"),
				ptr.To("destination-pvc"),
				ptr.To("data"),
				ptr.To("very-long-pvc-name-that-should-be-ignored"),
				nil,
			}

			expectedHostname := sanitizeForHostname(ns)

			for _, pvc := range pvcNames {
				// Source hostname
				sourceHostname := generateHostname(nil, pvc, ns, "source-obj")
				assert.Equal(t, expectedHostname, sourceHostname,
					"Source hostname should be namespace only for %s", ns)

				// Destination hostname
				destHostname := generateHostname(nil, pvc, ns, "dest-obj")
				assert.Equal(t, expectedHostname, destHostname,
					"Destination hostname should be namespace only for %s", ns)

				// They should match
				assert.Equal(t, sourceHostname, destHostname,
					"Source and destination hostnames should match for namespace %s", ns)
			}
		})
	}
}

// TestHostnameBackwardCompatibilityConsiderations documents the behavior change
// and its implications for existing snapshots
func TestHostnameBackwardCompatibilityConsiderations(t *testing.T) {
	// This test documents the migration considerations
	t.Run("document old vs new behavior", func(t *testing.T) {
		namespace := "production"
		pvcName := ptr.To("app-data")
		objectName := "backup-job"

		// New behavior: hostname is just namespace
		newHostname := generateHostname(nil, pvcName, namespace, objectName)
		assert.Equal(t, "production", newHostname,
			"New behavior: hostname should be namespace only")

		// Document what the old behavior would have been:
		// Old: "production-app-data" (when it fit within 50 chars)
		// New: "production"

		// This means existing snapshots with old hostnames like "production-app-data"
		// will not be accessible with the new hostname "production"
		// Users will need to either:
		// 1. Use custom hostname to match old format
		// 2. Create new snapshots with new hostname format
		// 3. Use a migration strategy if needed
	})

	t.Run("custom hostname can match old format", func(t *testing.T) {
		// Users can use custom hostname to access old snapshots
		customHostname := ptr.To("production-app-data")
		hostname := generateHostname(customHostname, ptr.To("app-data"), "production", "backup-job")
		assert.Equal(t, "production-app-data", hostname,
			"Custom hostname allows accessing old snapshots")
	})
}
