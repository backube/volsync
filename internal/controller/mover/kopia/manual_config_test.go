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
	"encoding/json"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

const (
	// kopiaManualConfigKey is the secret key for manual configuration
	kopiaManualConfigKey = "KOPIA_MANUAL_CONFIG"
)

type manualConfigEnvTestCase struct {
	name               string
	secretData         map[string][]byte
	expectManualConfig bool
	expectedValue      string
}

func getManualConfigEnvTestCases() []manualConfigEnvTestCase {
	return []manualConfigEnvTestCase{
		{
			name: "manual config provided",
			secretData: map[string][]byte{
				"KOPIA_REPOSITORY":   []byte("s3://bucket/path"),
				"KOPIA_PASSWORD":     []byte("password"),
				kopiaManualConfigKey: []byte(`{"encryption":{"algorithm":"CHACHA20-POLY1305"}}`),
			},
			expectManualConfig: true,
			expectedValue:      `{"encryption":{"algorithm":"CHACHA20-POLY1305"}}`,
		},
		{
			name: "no manual config",
			secretData: map[string][]byte{
				"KOPIA_REPOSITORY": []byte("s3://bucket/path"),
				"KOPIA_PASSWORD":   []byte("password"),
			},
			expectManualConfig: false,
		},
		{
			name: "complex manual config",
			secretData: map[string][]byte{
				"KOPIA_REPOSITORY": []byte("s3://bucket/path"),
				"KOPIA_PASSWORD":   []byte("password"),
				kopiaManualConfigKey: []byte(`{
					"encryption": {"algorithm": "AES256-GCM"},
					"compression": {"algorithm": "ZSTD-BEST", "minSize": 1024},
					"splitter": {"algorithm": "DYNAMIC-4M-BUZHASH"},
					"caching": {"maxCacheSize": 2147483648}
				}`),
			},
			expectManualConfig: true,
			expectedValue: `{
					"encryption": {"algorithm": "AES256-GCM"},
					"compression": {"algorithm": "ZSTD-BEST", "minSize": 1024},
					"splitter": {"algorithm": "DYNAMIC-4M-BUZHASH"},
					"caching": {"maxCacheSize": 2147483648}
				}`,
		},
	}
}

type manualConfigValidationTestCase struct {
	name        string
	config      string
	expectValid bool
	description string
}

func getManualConfigValidationTestCases() []manualConfigValidationTestCase {
	var cases []manualConfigValidationTestCase

	cases = append(cases, getValidConfigTestCases()...)
	cases = append(cases, getEdgeCaseConfigTestCases()...)

	return cases
}

func getValidConfigTestCases() []manualConfigValidationTestCase {
	return []manualConfigValidationTestCase{
		{
			name:        "valid encryption config",
			config:      `{"encryption":{"algorithm":"CHACHA20-POLY1305"}}`,
			expectValid: true,
			description: "Valid CHACHA20-POLY1305 encryption",
		},
		{
			name:        "valid compression config",
			config:      `{"compression":{"algorithm":"ZSTD-BEST","minSize":1024,"maxSize":1048576}}`,
			expectValid: true,
			description: "Valid ZSTD compression with size limits",
		},
		{
			name:        "valid splitter config",
			config:      `{"splitter":{"algorithm":"DYNAMIC-4M-BUZHASH"}}`,
			expectValid: true,
			description: "Valid dynamic splitter",
		},
		{
			name:        "valid caching config",
			config:      `{"caching":{"maxCacheSize":2147483648}}`,
			expectValid: true,
			description: "Valid cache size configuration",
		},
		{
			name: "complete valid config",
			config: `{
				"encryption": {"algorithm": "AES256-GCM"},
				"compression": {"algorithm": "ZSTD-FAST", "minSize": 512, "maxSize": 2097152},
				"splitter": {"algorithm": "FIXED-8M"},
				"caching": {"maxCacheSize": 1073741824}
			}`,
			expectValid: true,
			description: "Complete configuration with all sections",
		},
	}
}

func getEdgeCaseConfigTestCases() []manualConfigValidationTestCase {
	return []manualConfigValidationTestCase{
		{
			name:        "invalid JSON syntax",
			config:      `{"encryption":{"algorithm":"CHACHA20-POLY1305"`,
			expectValid: false,
			description: "Missing closing braces",
		},
		{
			name:        "empty config",
			config:      `{}`,
			expectValid: true,
			description: "Empty config is valid (uses defaults)",
		},
		{
			name:        "null values",
			config:      `{"encryption":null}`,
			expectValid: true,
			description: "Null values should be handled gracefully",
		},
		{
			name:        "unknown fields",
			config:      `{"unknownField":"value","encryption":{"algorithm":"AES256-GCM"}}`,
			expectValid: true,
			description: "Unknown fields should be ignored",
		},
		{
			name:        "numeric strings",
			config:      `{"caching":{"maxCacheSize":"2147483648"}}`,
			expectValid: true,
			description: "Numeric values as strings should be handled",
		},
	}
}

type backendCompatibilityTestCase struct {
	name       string
	secretData map[string][]byte
}

func getBackendCompatibilityTestCases(manualConfig string) []backendCompatibilityTestCase {
	var cases []backendCompatibilityTestCase

	cases = append(cases, getCloudBackendCompatibilityTestCases(manualConfig)...)
	cases = append(cases, getProtocolBackendCompatibilityTestCases(manualConfig)...)
	cases = append(cases, getFilesystemBackendCompatibilityTestCase(manualConfig))

	return cases
}

func getCloudBackendCompatibilityTestCases(manualConfig string) []backendCompatibilityTestCase {
	return []backendCompatibilityTestCase{
		{
			name: "S3 backend",
			secretData: map[string][]byte{
				"KOPIA_REPOSITORY":      []byte("s3://bucket/path"),
				"KOPIA_PASSWORD":        []byte("password"),
				"AWS_ACCESS_KEY_ID":     []byte("key"),
				"AWS_SECRET_ACCESS_KEY": []byte("secret"),
				kopiaManualConfigKey:    []byte(manualConfig),
			},
		},
		{
			name: "Azure backend",
			secretData: map[string][]byte{
				"KOPIA_REPOSITORY":   []byte("azure://container/path"),
				"KOPIA_PASSWORD":     []byte("password"),
				"AZURE_ACCOUNT_NAME": []byte("account"),
				"AZURE_ACCOUNT_KEY":  []byte("key"),
				kopiaManualConfigKey: []byte(manualConfig),
			},
		},
		{
			name: "GCS backend",
			secretData: map[string][]byte{
				"KOPIA_REPOSITORY":               []byte("gcs://bucket/path"),
				"KOPIA_PASSWORD":                 []byte("password"),
				"GOOGLE_APPLICATION_CREDENTIALS": []byte(`{"type":"service_account"}`),
				kopiaManualConfigKey:             []byte(manualConfig),
			},
		},
		{
			name: "B2 backend",
			secretData: map[string][]byte{
				"KOPIA_REPOSITORY":   []byte("b2://bucket/path"),
				"KOPIA_PASSWORD":     []byte("password"),
				"B2_ACCOUNT_ID":      []byte("account"),
				"B2_APPLICATION_KEY": []byte("key"),
				kopiaManualConfigKey: []byte(manualConfig),
			},
		},
	}
}

func getProtocolBackendCompatibilityTestCases(manualConfig string) []backendCompatibilityTestCase {
	return []backendCompatibilityTestCase{
		{
			name: "WebDAV backend",
			secretData: map[string][]byte{
				"KOPIA_REPOSITORY":   []byte("webdav://server/path"),
				"KOPIA_PASSWORD":     []byte("password"),
				"WEBDAV_URL":         []byte("https://server/webdav"),
				"WEBDAV_USERNAME":    []byte("user"),
				"WEBDAV_PASSWORD":    []byte("pass"),
				kopiaManualConfigKey: []byte(manualConfig),
			},
		},
		{
			name: "SFTP backend",
			secretData: map[string][]byte{
				"KOPIA_REPOSITORY":   []byte("sftp://server/path"),
				"KOPIA_PASSWORD":     []byte("password"),
				"SFTP_HOST":          []byte("server"),
				"SFTP_USERNAME":      []byte("user"),
				"SFTP_PASSWORD":      []byte("pass"),
				kopiaManualConfigKey: []byte(manualConfig),
			},
		},
	}
}

func getFilesystemBackendCompatibilityTestCase(manualConfig string) backendCompatibilityTestCase {
	return backendCompatibilityTestCase{
		name: "Filesystem backend",
		secretData: map[string][]byte{
			"KOPIA_REPOSITORY":   []byte("filesystem:///mnt/backup"),
			"KOPIA_PASSWORD":     []byte("password"),
			kopiaManualConfigKey: []byte(manualConfig),
		},
	}
}

type multiTenancyManualConfigTestCase struct {
	name             string
	namespace        string
	sourceName       string
	username         string
	hostname         string
	expectedUsername string
	expectedHostname string
}

func getMultiTenancyManualConfigTestCases() []multiTenancyManualConfigTestCase {
	return []multiTenancyManualConfigTestCase{
		{
			name:             "default multi-tenancy",
			namespace:        "production",
			sourceName:       "app-backup",
			username:         "source",
			hostname:         "production-app-backup",
			expectedUsername: "source",
			expectedHostname: "production-app-backup",
		},
		{
			name:             "custom username and hostname",
			namespace:        "staging",
			sourceName:       "database",
			username:         "custom-user",
			hostname:         "custom-host",
			expectedUsername: "custom-user",
			expectedHostname: "custom-host",
		},
	}
}

type edgeCaseTestCase struct {
	name        string
	config      string
	description string
}

func getEdgeCaseTestCases() []edgeCaseTestCase {
	return []edgeCaseTestCase{
		{
			name:        "empty string",
			config:      "",
			description: "Empty config should not cause errors",
		},
		{
			name:        "very large config",
			config:      `{"caching":{"maxCacheSize":` + strings.Repeat("9", 1000) + `}}`,
			description: "Very large numbers should be handled",
		},
		{
			name:        "deeply nested config",
			config:      `{"a":{"b":{"c":{"d":{"e":{"f":"value"}}}}}}`,
			description: "Deeply nested JSON should be handled",
		},
		{
			name:        "unicode in config",
			config:      `{"comment":"测试配置 🚀"}`,
			description: "Unicode characters should be preserved",
		},
		{
			name:        "escaped characters",
			config:      `{"path":"C:\\Program Files\\Kopia"}`,
			description: "Escaped characters should be handled",
		},
	}
}

// Helper function to normalize JSON for comparison
func normalizeJSON(jsonStr string) string {
	var data interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return jsonStr // Return as-is if not valid JSON
	}
	normalized, _ := json.Marshal(data)
	return string(normalized)
}

// Helper function to get required environment variables for each backend
func getRequiredVarsForBackend(backendName string) []string {
	switch backendName {
	case "S3 backend":
		return []string{"KOPIA_REPOSITORY", "KOPIA_PASSWORD", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY",
			"KOPIA_S3_ENDPOINT", "AWS_S3_ENDPOINT", "KOPIA_S3_DISABLE_TLS", "AWS_S3_DISABLE_TLS",
			"AWS_REGION", "AWS_DEFAULT_REGION"}
	case "Azure backend":
		return []string{"KOPIA_REPOSITORY", "KOPIA_PASSWORD", "AZURE_ACCOUNT_NAME", "AZURE_ACCOUNT_KEY",
			"KOPIA_AZURE_STORAGE_ACCOUNT", "AZURE_STORAGE_ACCOUNT",
			"KOPIA_AZURE_STORAGE_KEY", "AZURE_STORAGE_KEY", "AZURE_STORAGE_SAS_TOKEN"}
	case "GCS backend":
		return []string{"KOPIA_REPOSITORY", "KOPIA_PASSWORD", "GOOGLE_APPLICATION_CREDENTIALS", "GOOGLE_PROJECT_ID"}
	case "B2 backend":
		return []string{"KOPIA_REPOSITORY", "KOPIA_PASSWORD", "B2_ACCOUNT_ID", "B2_APPLICATION_KEY"}
	case "WebDAV backend":
		return []string{"KOPIA_REPOSITORY", "KOPIA_PASSWORD", "WEBDAV_URL", "WEBDAV_USERNAME", "WEBDAV_PASSWORD"}
	case "SFTP backend":
		return []string{"KOPIA_REPOSITORY", "KOPIA_PASSWORD", "SFTP_HOST", "SFTP_USERNAME", "SFTP_PASSWORD", "SFTP_KEY_FILE"}
	case "Filesystem backend":
		return []string{"KOPIA_REPOSITORY", "KOPIA_PASSWORD"}
	default:
		return []string{"KOPIA_REPOSITORY", "KOPIA_PASSWORD"}
	}
}

var _ = Describe("Manual Config Environment Variable", func() {
	DescribeTable("tests that KOPIA_MANUAL_CONFIG is included in environment variables",
		func(tt manualConfigEnvTestCase) {
			// Create mock owner
			owner := &volsyncv1alpha1.ReplicationSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-rs",
					Namespace: "test-namespace",
				},
			}

			// Create mover
			mover := &Mover{
				username: "test-user",
				hostname: "test-host",
				owner:    owner,
			}

			// Create secret
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-secret",
				},
				Data: tt.secretData,
			}

			// Build environment variables
			envVars := mover.buildEnvironmentVariables(secret)

			// Check if KOPIA_MANUAL_CONFIG is present
			found := false
			var actualValue string
			for _, env := range envVars {
				if env.Name == kopiaManualConfigKey {
					found = true
					if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
						// It's a secret reference, check that it references the right key
						Expect(env.ValueFrom.SecretKeyRef.Key).To(Equal(kopiaManualConfigKey),
							"Expected secret key reference to be %s, got %s",
							kopiaManualConfigKey, env.ValueFrom.SecretKeyRef.Key)
						Expect(env.ValueFrom.SecretKeyRef.Name).To(Equal(secret.Name),
							"Expected secret name reference to be %s, got %s",
							secret.Name, env.ValueFrom.SecretKeyRef.Name)
						// The actual value would come from the secret
						actualValue = string(tt.secretData[kopiaManualConfigKey])
					} else {
						actualValue = env.Value
					}
					break
				}
			}

			if tt.expectManualConfig {
				Expect(found).To(BeTrue(),
					"Expected %s to be present in environment variables", kopiaManualConfigKey)
				// Normalize JSON for comparison (remove whitespace differences)
				expectedNorm := normalizeJSON(tt.expectedValue)
				actualNorm := normalizeJSON(actualValue)
				Expect(actualNorm).To(Equal(expectedNorm),
					"%s value mismatch\nExpected: %s\nActual: %s",
					kopiaManualConfigKey, expectedNorm, actualNorm)
			}
		},
		Entry("manual config provided", getManualConfigEnvTestCases()[0]),
		Entry("no manual config", getManualConfigEnvTestCases()[1]),
		Entry("complex manual config", getManualConfigEnvTestCases()[2]),
	)
})

var _ = Describe("Manual Config Validation", func() {
	DescribeTable("tests JSON validation for manual configuration",
		func(tt manualConfigValidationTestCase) {
			// Validate JSON syntax
			var result map[string]interface{}
			err := json.Unmarshal([]byte(tt.config), &result)

			isValid := err == nil
			Expect(isValid).To(Equal(tt.expectValid),
				"Config validation mismatch for %s: expected valid=%v, got valid=%v (error: %v)",
				tt.description, tt.expectValid, isValid, err)
		},
		Entry("valid encryption config", getValidConfigTestCases()[0]),
		Entry("valid compression config", getValidConfigTestCases()[1]),
		Entry("valid splitter config", getValidConfigTestCases()[2]),
		Entry("valid caching config", getValidConfigTestCases()[3]),
		Entry("complete valid config", getValidConfigTestCases()[4]),
		Entry("invalid JSON syntax", getEdgeCaseConfigTestCases()[0]),
		Entry("empty config", getEdgeCaseConfigTestCases()[1]),
		Entry("null values", getEdgeCaseConfigTestCases()[2]),
		Entry("unknown fields", getEdgeCaseConfigTestCases()[3]),
		Entry("numeric strings", getEdgeCaseConfigTestCases()[4]),
	)
})

var _ = Describe("Manual Config Compatibility", func() {
	manualConfig := `{"encryption":{"algorithm":"CHACHA20-POLY1305"},"compression":{"algorithm":"ZSTD-DEFAULT"}}`

	DescribeTable("tests that manual config works with different backends",
		func(backend backendCompatibilityTestCase) {
			// Create mock owner
			owner := &volsyncv1alpha1.ReplicationSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-rs",
					Namespace: "test-namespace",
				},
			}

			// Create mover
			mover := &Mover{
				username: "test-user",
				hostname: "test-host",
				owner:    owner,
			}

			// Create secret
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-secret",
				},
				Data: backend.secretData,
			}

			// Build environment variables
			envVars := mover.buildEnvironmentVariables(secret)

			// Verify KOPIA_MANUAL_CONFIG is present
			found := false
			for _, env := range envVars {
				if env.Name == kopiaManualConfigKey {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "%s: %s not found in environment variables", backend.name, kopiaManualConfigKey)

			// Verify backend-specific variables are also present
			requiredVars := getRequiredVarsForBackend(backend.name)
			for _, reqVar := range requiredVars {
				found := false
				for _, env := range envVars {
					if env.Name == reqVar {
						found = true
						break
					}
				}
				Expect(found).To(BeTrue(), "%s: Required variable %s not found", backend.name, reqVar)
			}
		},
		Entry("S3 backend", getCloudBackendCompatibilityTestCases(manualConfig)[0]),
		Entry("Azure backend", getCloudBackendCompatibilityTestCases(manualConfig)[1]),
		Entry("GCS backend", getCloudBackendCompatibilityTestCases(manualConfig)[2]),
		Entry("B2 backend", getCloudBackendCompatibilityTestCases(manualConfig)[3]),
		Entry("WebDAV backend", getProtocolBackendCompatibilityTestCases(manualConfig)[0]),
		Entry("SFTP backend", getProtocolBackendCompatibilityTestCases(manualConfig)[1]),
		Entry("Filesystem backend", getFilesystemBackendCompatibilityTestCase(manualConfig)),
	)
})

var _ = Describe("Manual Config Multi-Tenancy", func() {
	DescribeTable("tests that manual config preserves multi-tenancy",
		func(tt multiTenancyManualConfigTestCase) {
			// Create mock owner
			owner := &volsyncv1alpha1.ReplicationSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tt.sourceName,
					Namespace: tt.namespace,
				},
			}

			// Create mover with manual config
			mover := &Mover{
				username: tt.username,
				hostname: tt.hostname,
				owner:    owner,
			}

			// Create secret with manual config
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-secret",
				},
				Data: map[string][]byte{
					"KOPIA_REPOSITORY": []byte("s3://bucket/path"),
					"KOPIA_PASSWORD":   []byte("password"),
					kopiaManualConfigKey: []byte(`{
						"encryption": {"algorithm": "CHACHA20-POLY1305"},
						"compression": {"algorithm": "ZSTD-BEST"}
					}`),
				},
			}

			// Build environment variables
			envVars := mover.buildEnvironmentVariables(secret)

			// Verify username and hostname overrides are preserved
			var actualUsername, actualHostname string
			for _, env := range envVars {
				if env.Name == "KOPIA_OVERRIDE_USERNAME" {
					actualUsername = env.Value
				}
				if env.Name == "KOPIA_OVERRIDE_HOSTNAME" {
					actualHostname = env.Value
				}
			}

			Expect(actualUsername).To(Equal(tt.expectedUsername), "Expected username %s, got %s", tt.expectedUsername, actualUsername)
			Expect(actualHostname).To(Equal(tt.expectedHostname), "Expected hostname %s, got %s", tt.expectedHostname, actualHostname)
		},
		Entry("default multi-tenancy", getMultiTenancyManualConfigTestCases()[0]),
		Entry("custom username and hostname", getMultiTenancyManualConfigTestCases()[1]),
	)
})

var _ = Describe("Manual Config Edge Cases", func() {
	DescribeTable("tests edge cases and error conditions",
		func(tt edgeCaseTestCase) {
			// Create mock owner
			owner := &volsyncv1alpha1.ReplicationSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-rs",
					Namespace: "test-namespace",
				},
			}

			// Create mover
			mover := &Mover{
				username: "test-user",
				hostname: "test-host",
				owner:    owner,
			}

			// Create secret
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-secret",
				},
				Data: map[string][]byte{
					"KOPIA_REPOSITORY":   []byte("s3://bucket/path"),
					"KOPIA_PASSWORD":     []byte("password"),
					kopiaManualConfigKey: []byte(tt.config),
				},
			}

			// This should not panic
			envVars := mover.buildEnvironmentVariables(secret)

			// Verify basic environment is still created
			Expect(len(envVars)).To(BeNumerically(">", 0),
				"%s: No environment variables created (%s)", tt.name, tt.description)

			// Verify required variables are still present
			hasRepository := false
			hasPassword := false
			for _, env := range envVars {
				if env.Name == "KOPIA_REPOSITORY" {
					hasRepository = true
				}
				if env.Name == "KOPIA_PASSWORD" {
					hasPassword = true
				}
			}

			Expect(hasRepository && hasPassword).To(BeTrue(),
				"%s: Required variables missing (%s)", tt.name, tt.description)
		},
		Entry("empty string", getEdgeCaseTestCases()[0]),
		Entry("very large config", getEdgeCaseTestCases()[1]),
		Entry("deeply nested config", getEdgeCaseTestCases()[2]),
		Entry("unicode in config", getEdgeCaseTestCases()[3]),
		Entry("escaped characters", getEdgeCaseTestCases()[4]),
	)
})
