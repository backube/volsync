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
	"k8s.io/utils/ptr"
)

// hostnameTestCase represents a test case for hostname generation
type hostnameTestCase struct {
	hostname    *string
	pvcName     *string
	namespace   string
	objectName  string
	expected    string
	description string
}

var _ = Describe("Hostname Is Always Namespace Only", func() {
	DescribeTable("verifies that hostname generation ALWAYS uses only the namespace, never including PVC names",
		func(tc hostnameTestCase) {
			result := generateHostname(tc.hostname, tc.pvcName, tc.namespace, tc.objectName)
			Expect(result).To(Equal(tc.expected), tc.description)

			// Additional assertion: if namespace is valid and no custom hostname,
			// the result should NEVER contain the PVC name
			if tc.hostname == nil && tc.pvcName != nil && *tc.pvcName != "" && tc.namespace != "" {
				sanitizedNamespace := sanitizeForHostname(tc.namespace)
				if sanitizedNamespace != "" {
					// The result should be based on namespace, not containing PVC parts
					Expect(result).NotTo(ContainSubstring(sanitizeForHostname(*tc.pvcName)),
						"Hostname should not contain PVC name when namespace is valid")
				}
			}
		},
		// Basic namespace-only behavior
		Entry("simple namespace", hostnameTestCase{
			hostname:    nil,
			pvcName:     nil,
			namespace:   "production",
			objectName:  "backup-job",
			expected:    "production",
			description: "Should use namespace only",
		}),
		Entry("namespace with PVC - PVC ignored", hostnameTestCase{
			hostname:    nil,
			pvcName:     ptr.To("my-data-pvc"),
			namespace:   "production",
			objectName:  "backup-job",
			expected:    "production",
			description: "Should use namespace only, ignoring PVC",
		}),
		Entry("namespace with long PVC - PVC ignored", hostnameTestCase{
			hostname:    nil,
			pvcName:     ptr.To("very-long-persistent-volume-claim-name-that-would-exceed-limits"),
			namespace:   "prod",
			objectName:  "backup-job",
			expected:    "prod",
			description: "Should use namespace only, ignoring long PVC",
		}),
		Entry("namespace with special characters in PVC", hostnameTestCase{
			hostname:    nil,
			pvcName:     ptr.To("my_special@pvc.name"),
			namespace:   "staging",
			objectName:  "backup-job",
			expected:    "staging",
			description: "Should use namespace only, ignoring PVC with special chars",
		}),
		// Namespace sanitization tests
		Entry("namespace with underscores", hostnameTestCase{
			hostname:    nil,
			pvcName:     ptr.To("data-pvc"),
			namespace:   "test_namespace",
			objectName:  "backup-job",
			expected:    "test-namespace",
			description: "Should sanitize namespace (convert underscores) and ignore PVC",
		}),
		Entry("namespace with dots", hostnameTestCase{
			hostname:    nil,
			pvcName:     ptr.To("data-pvc"),
			namespace:   "prod.region.cluster",
			objectName:  "backup-job",
			expected:    "prod.region.cluster",
			description: "Should preserve dots in namespace and ignore PVC",
		}),
		Entry("namespace with special characters", hostnameTestCase{
			hostname:    nil,
			pvcName:     ptr.To("data-pvc"),
			namespace:   "test@namespace#123",
			objectName:  "backup-job",
			expected:    "testnamespace123",
			description: "Should sanitize namespace and ignore PVC",
		}),
		// Multi-tenant scenarios
		Entry("tenant-a namespace", hostnameTestCase{
			hostname:    nil,
			pvcName:     ptr.To("tenant-a-database"),
			namespace:   "tenant-a",
			objectName:  "backup-job",
			expected:    "tenant-a",
			description: "Multi-tenant: should use tenant namespace only",
		}),
		Entry("tenant-b namespace", hostnameTestCase{
			hostname:    nil,
			pvcName:     ptr.To("tenant-b-database"),
			namespace:   "tenant-b",
			objectName:  "backup-job",
			expected:    "tenant-b",
			description: "Multi-tenant: should use tenant namespace only",
		}),
		Entry("customer namespace with app PVC", hostnameTestCase{
			hostname:    nil,
			pvcName:     ptr.To("customer-app-data"),
			namespace:   "customer-prod",
			objectName:  "backup-job",
			expected:    "customer-prod",
			description: "Multi-tenant: should use customer namespace only",
		}),
		// Namespace length tests
		Entry("very long namespace", hostnameTestCase{
			hostname:    nil,
			pvcName:     ptr.To("data"),
			namespace:   "very-long-namespace-name-that-is-still-valid-in-kubernetes",
			objectName:  "backup-job",
			expected:    "very-long-namespace-name-that-is-still-valid-in-kubernetes",
			description: "Should use full namespace even if long, ignore PVC",
		}),
		Entry("short namespace", hostnameTestCase{
			hostname:    nil,
			pvcName:     ptr.To("very-long-pvc-name"),
			namespace:   "ns",
			objectName:  "backup-job",
			expected:    "ns",
			description: "Should use short namespace, ignore PVC",
		}),
		// PVC edge cases
		Entry("empty PVC name", hostnameTestCase{
			hostname:    nil,
			pvcName:     ptr.To(""),
			namespace:   "namespace",
			objectName:  "backup-job",
			expected:    "namespace",
			description: "Should use namespace when PVC is empty",
		}),
		Entry("nil PVC", hostnameTestCase{
			hostname:    nil,
			pvcName:     nil,
			namespace:   "namespace",
			objectName:  "backup-job",
			expected:    "namespace",
			description: "Should use namespace when PVC is nil",
		}),
		// Fallback tests
		Entry("custom hostname overrides everything", hostnameTestCase{
			hostname:    ptr.To("my-custom-hostname"),
			pvcName:     ptr.To("data-pvc"),
			namespace:   "production",
			objectName:  "backup-job",
			expected:    "my-custom-hostname",
			description: "Custom hostname should override namespace-only logic",
		}),
		Entry("invalid namespace falls back", hostnameTestCase{
			hostname:    nil,
			pvcName:     ptr.To("data-pvc"),
			namespace:   "@#$%^&*()",
			objectName:  "backup-job",
			expected:    "backup-job",
			description: "Should fallback when namespace is invalid",
		}),
		Entry("empty namespace falls back", hostnameTestCase{
			hostname:    nil,
			pvcName:     ptr.To("data-pvc"),
			namespace:   "",
			objectName:  "backup-job",
			expected:    "backup-job",
			description: "Should fallback when namespace is empty",
		}),
	)
})

var _ = Describe("Hostname Consistency Across Source And Destination", func() {
	DescribeTable("verifies that the same namespace always generates the same hostname for both ReplicationSource and ReplicationDestination",
		func(ns string) {
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
				Expect(sourceHostname).To(Equal(expectedHostname),
					"Source hostname should be namespace only for %s", ns)

				// Destination hostname
				destHostname := generateHostname(nil, pvc, ns, "dest-obj")
				Expect(destHostname).To(Equal(expectedHostname),
					"Destination hostname should be namespace only for %s", ns)

				// They should match
				Expect(sourceHostname).To(Equal(destHostname),
					"Source and destination hostnames should match for namespace %s", ns)
			}
		},
		Entry("production", "production"),
		Entry("staging", "staging"),
		Entry("development", "development"),
		Entry("tenant-a", "tenant-a"),
		Entry("tenant-b", "tenant-b"),
		Entry("customer-prod", "customer-prod"),
		Entry("test-namespace", "test-namespace"),
		Entry("my_namespace", "my_namespace"),
		Entry("namespace.with.dots", "namespace.with.dots"),
	)
})

var _ = Describe("Hostname Backward Compatibility Considerations", func() {
	// This test documents the migration considerations
	Context("when documenting old vs new behavior", func() {
		It("should show that new behavior uses namespace only", func() {
			namespace := "production"
			pvcName := ptr.To("app-data")
			objectName := "backup-job"

			// New behavior: hostname is just namespace
			newHostname := generateHostname(nil, pvcName, namespace, objectName)
			Expect(newHostname).To(Equal("production"),
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
	})

	Context("when using custom hostname to match old format", func() {
		It("should allow users to access old snapshots", func() {
			// Users can use custom hostname to access old snapshots
			customHostname := ptr.To("production-app-data")
			hostname := generateHostname(customHostname, ptr.To("app-data"), "production", "backup-job")
			Expect(hostname).To(Equal("production-app-data"),
				"Custom hostname allows accessing old snapshots")
		})
	})
})
