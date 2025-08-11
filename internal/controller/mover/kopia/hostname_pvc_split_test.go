//go:build !disable_kopia

package kopia

import (
	"strings"

	"k8s.io/utils/ptr"
)

// Split helper functions for hostname test cases

func getHostnameWithPVCTestCases() []hostnameWithPVCTestCase {
	var cases []hostnameWithPVCTestCase
	cases = append(cases, getCustomHostnameCases()...)
	cases = append(cases, getNamespaceWithPVCCases()...)
	cases = append(cases, getEdgeAndFallbackCases()...)
	return cases
}

func getCustomHostnameCases() []hostnameWithPVCTestCase {
	return []hostnameWithPVCTestCase{
		{
			name:        "custom hostname provided",
			hostname:    ptr.To("custom-host"),
			pvcName:     ptr.To("my-pvc"),
			namespace:   "test-ns",
			objectName:  "test-obj",
			expected:    "custom-host",
			description: "Should return custom hostname when provided, ignoring namespace and PVC",
		},
	}
}

func getNamespaceWithPVCCases() []hostnameWithPVCTestCase {
	return []hostnameWithPVCTestCase{
		{
			name:        "namespace with PVC - ignore PVC",
			hostname:    nil,
			pvcName:     ptr.To("app-data"),
			namespace:   "production",
			objectName:  "test-obj",
			expected:    "production",
			description: "Should only use namespace, ignoring PVC name",
		},
		{
			name:        "namespace with PVC - sanitization",
			hostname:    nil,
			pvcName:     ptr.To("data_pvc_volume"),
			namespace:   "test_ns",
			objectName:  "test-obj",
			expected:    "test-ns",
			description: "Should sanitize namespace and ignore PVC name",
		},
		{
			name:        "namespace with PVC - special characters",
			hostname:    nil,
			pvcName:     ptr.To("data@pvc.storage"),
			namespace:   "prod@env",
			objectName:  "test-obj",
			expected:    "prodenv",
			description: "Should remove special characters from namespace and ignore PVC",
		},
		{
			name:        "namespace with PVC - long PVC ignored",
			hostname:    nil,
			pvcName:     ptr.To(strings.Repeat("very-long-pvc-name", 3)),
			namespace:   "my-namespace",
			objectName:  "test-obj",
			expected:    "my-namespace",
			description: "Should use only namespace, ignoring long PVC name",
		},
		{
			name:        "namespace only - no PVC",
			hostname:    nil,
			pvcName:     nil,
			namespace:   "test-namespace",
			objectName:  "test-obj",
			expected:    "test-namespace",
			description: "Should use namespace when PVC is nil",
		},
		{
			name:        "namespace only - empty PVC",
			hostname:    nil,
			pvcName:     ptr.To(""),
			namespace:   "test-namespace",
			objectName:  "test-obj",
			expected:    "test-namespace",
			description: "Should use namespace when PVC is empty string",
		},
	}
}

func getEdgeAndFallbackCases() []hostnameWithPVCTestCase {
	var cases []hostnameWithPVCTestCase
	cases = append(cases, getSanitizationEdgeCases()...)
	cases = append(cases, getLengthEdgeCases()...)
	cases = append(cases, getFallbackCases()...)
	return cases
}

func getSanitizationEdgeCases() []hostnameWithPVCTestCase {
	return []hostnameWithPVCTestCase{
		{
			name:        "namespace with leading and trailing special chars",
			hostname:    nil,
			pvcName:     ptr.To("-_data-pvc_-"),
			namespace:   "-_test-ns_-",
			objectName:  "test-obj",
			expected:    "test-ns",
			description: "Should trim leading and trailing hyphens from namespace and ignore PVC",
		},
		{
			name:        "PVC becomes empty after sanitization",
			hostname:    nil,
			pvcName:     ptr.To("@#$%^&*()"),
			namespace:   "test-ns",
			objectName:  "test-obj",
			expected:    "test-ns",
			description: "Should use only namespace, ignoring invalid PVC",
		},
		{
			name:        "namespace becomes empty - fallback to namespace-name",
			hostname:    nil,
			pvcName:     ptr.To("data-pvc"),
			namespace:   "@#$%^&*()",
			objectName:  "test-obj",
			expected:    "test-obj",
			description: "Should fallback to namespace-name format when namespace is invalid",
		},
	}
}

func getLengthEdgeCases() []hostnameWithPVCTestCase {
	var cases []hostnameWithPVCTestCase
	cases = append(cases, getNamespaceLengthCases()...)
	cases = append(cases, getCharacterLimitCases()...)
	cases = append(cases, getDotHandlingCases()...)
	return cases
}

func getNamespaceLengthCases() []hostnameWithPVCTestCase {
	return []hostnameWithPVCTestCase{
		{
			name:        "long namespace only",
			hostname:    nil,
			pvcName:     nil,
			namespace:   strings.Repeat("long-namespace-", 5),
			objectName:  "test-obj",
			expected:    strings.Repeat("long-namespace-", 5)[:len(strings.Repeat("long-namespace-", 5))-1],
			description: "Should handle long namespace correctly",
		},
		{
			name:        "namespace with mixed case",
			hostname:    nil,
			pvcName:     ptr.To("MyApp-Data-PVC"),
			namespace:   "MyNamespace",
			objectName:  "test-obj",
			expected:    "MyNamespace",
			description: "Should preserve case in namespace and ignore PVC",
		},
		{
			name:        "namespace with numbers",
			hostname:    nil,
			pvcName:     ptr.To("pvc-123"),
			namespace:   "namespace-456",
			objectName:  "test-obj",
			expected:    "namespace-456",
			description: "Should handle numbers in namespace and ignore PVC",
		},
		{
			name:        "short namespace with PVC ignored",
			hostname:    nil,
			pvcName:     ptr.To("very-long-persistent-volume-claim-name-that-is-really-long"),
			namespace:   "ns",
			objectName:  "test-obj",
			expected:    "ns",
			description: "Should use namespace even when very short, ignoring PVC",
		},
	}
}

func getCharacterLimitCases() []hostnameWithPVCTestCase {
	return []hostnameWithPVCTestCase{
		{
			name:        "namespace with PVC ignored",
			hostname:    nil,
			pvcName:     ptr.To("data"),
			namespace:   strings.Repeat("a", 45),
			objectName:  "test-obj",
			expected:    strings.Repeat("a", 45),
			description: "Should use namespace only, ignoring PVC",
		},
		{
			name:        "namespace with long PVC ignored",
			hostname:    nil,
			pvcName:     ptr.To("data1"),
			namespace:   strings.Repeat("a", 45),
			objectName:  "test-obj",
			expected:    strings.Repeat("a", 45),
			description: "Should use namespace only, ignoring PVC",
		},
	}
}

func getDotHandlingCases() []hostnameWithPVCTestCase {
	return []hostnameWithPVCTestCase{
		{
			name:        "PVC with dots ignored",
			hostname:    nil,
			pvcName:     ptr.To("my.app.pvc"),
			namespace:   "production",
			objectName:  "test-obj",
			expected:    "production",
			description: "Should use namespace only, ignoring PVC with dots",
		},
		{
			name:        "namespace with dots",
			hostname:    nil,
			pvcName:     ptr.To("data-pvc"),
			namespace:   "prod.region.cluster",
			objectName:  "test-obj",
			expected:    "prod.region.cluster",
			description: "Should preserve dots in namespace and ignore PVC",
		},
	}
}

func getFallbackCases() []hostnameWithPVCTestCase {
	return []hostnameWithPVCTestCase{
		{
			name:        "trailing dots and hyphens",
			hostname:    nil,
			pvcName:     ptr.To("data-pvc..."),
			namespace:   "test-ns---",
			objectName:  "test-obj",
			expected:    "test-ns",
			description: "Should trim trailing dots and hyphens from namespace, ignore PVC",
		},
		{
			name:        "both namespace and PVC invalid - use object name",
			hostname:    nil,
			pvcName:     ptr.To("@#$%^&*()"),
			namespace:   "@#$%^&*()",
			objectName:  "test-obj",
			expected:    "test-obj",
			description: "Should use sanitized object name when namespace is invalid",
		},
		{
			name:        "everything invalid - use default",
			hostname:    nil,
			pvcName:     ptr.To("@#$%^&*()"),
			namespace:   "@#$%^&*()",
			objectName:  "@#$%^&*()",
			expected:    defaultUsername,
			description: "Should use volsync-default when everything is invalid",
		},
	}
}
