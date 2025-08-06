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
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

const (
	// kopiaManualConfigKey is the secret key for manual configuration
	kopiaManualConfigKey = "KOPIA_MANUAL_CONFIG"
)

// TestManualConfigEnvironmentVariable tests that KOPIA_MANUAL_CONFIG is included in environment variables
func TestManualConfigEnvironmentVariable(t *testing.T) {
	tests := getManualConfigEnvTestCases()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testManualConfigEnvironment(t, tt)
		})
	}
}

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

func testManualConfigEnvironment(t *testing.T, tt manualConfigEnvTestCase) {
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
	verifyManualConfigPresence(t, tt, secret, envVars)
}

func verifyManualConfigPresence(
	t *testing.T,
	tt manualConfigEnvTestCase,
	secret *corev1.Secret,
	envVars []corev1.EnvVar,
) {
	found := false
	var actualValue string
	for _, env := range envVars {
		if env.Name == kopiaManualConfigKey {
			found = true
			if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
				// It's a secret reference, check that it references the right key
				if env.ValueFrom.SecretKeyRef.Key != kopiaManualConfigKey {
					t.Errorf("Expected secret key reference to be %s, got %s",
						kopiaManualConfigKey, env.ValueFrom.SecretKeyRef.Key)
				}
				if env.ValueFrom.SecretKeyRef.Name != secret.Name {
					t.Errorf("Expected secret name reference to be %s, got %s",
						secret.Name, env.ValueFrom.SecretKeyRef.Name)
				}
				// The actual value would come from the secret
				actualValue = string(tt.secretData[kopiaManualConfigKey])
			} else {
				actualValue = env.Value
			}
			break
		}
	}

	if tt.expectManualConfig && !found {
		t.Errorf("Expected %s to be present in environment variables", kopiaManualConfigKey)
	}

	if tt.expectManualConfig && found {
		// Normalize JSON for comparison (remove whitespace differences)
		expectedNorm := normalizeJSON(tt.expectedValue)
		actualNorm := normalizeJSON(actualValue)
		if expectedNorm != actualNorm {
			t.Errorf("%s value mismatch\nExpected: %s\nActual: %s",
				kopiaManualConfigKey, expectedNorm, actualNorm)
		}
	}
}

// TestManualConfigValidation tests JSON validation for manual configuration
func TestManualConfigValidation(t *testing.T) {
	tests := getManualConfigValidationTestCases()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testManualConfigValidation(t, tt)
		})
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

func testManualConfigValidation(t *testing.T, tt manualConfigValidationTestCase) {
	// Validate JSON syntax
	var result map[string]interface{}
	err := json.Unmarshal([]byte(tt.config), &result)

	isValid := err == nil
	if isValid != tt.expectValid {
		if tt.expectValid {
			t.Errorf("Expected config to be valid (%s), but got error: %v", tt.description, err)
		} else {
			t.Errorf("Expected config to be invalid (%s), but it was accepted", tt.description)
		}
	}
}

// TestManualConfigCompatibility tests that manual config works with different backends
func TestManualConfigCompatibility(t *testing.T) {
	manualConfig := `{"encryption":{"algorithm":"CHACHA20-POLY1305"},"compression":{"algorithm":"ZSTD-DEFAULT"}}`
	backends := getBackendCompatibilityTestCases(manualConfig)

	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			testManualConfigCompatibility(t, backend)
		})
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
			"KOPIA_FS_PATH":      []byte("/mnt/backup"),
			kopiaManualConfigKey: []byte(manualConfig),
		},
	}
}

func testManualConfigCompatibility(t *testing.T, backend backendCompatibilityTestCase) {
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
	verifyManualConfigInEnvironment(t, backend.name, envVars)

	// Verify backend-specific variables are also present
	verifyBackendSpecificVariables(t, backend.name, envVars)
}

func verifyManualConfigInEnvironment(t *testing.T, backendName string, envVars []corev1.EnvVar) {
	found := false
	for _, env := range envVars {
		if env.Name == kopiaManualConfigKey {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("%s: %s not found in environment variables", backendName, kopiaManualConfigKey)
	}
}

func verifyBackendSpecificVariables(t *testing.T, backendName string, envVars []corev1.EnvVar) {
	requiredVars := getRequiredVarsForBackend(backendName)
	for _, reqVar := range requiredVars {
		found := false
		for _, env := range envVars {
			if env.Name == reqVar {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s: Required variable %s not found", backendName, reqVar)
		}
	}
}

// TestManualConfigMultiTenancy tests that manual config preserves multi-tenancy
func TestManualConfigMultiTenancy(t *testing.T) {
	tests := getMultiTenancyManualConfigTestCases()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testManualConfigMultiTenancy(t, tt)
		})
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

func testManualConfigMultiTenancy(t *testing.T, tt multiTenancyManualConfigTestCase) {
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
	verifyMultiTenancyOverrides(t, tt.expectedUsername, tt.expectedHostname, envVars)
}

func verifyMultiTenancyOverrides(t *testing.T, expectedUsername, expectedHostname string, envVars []corev1.EnvVar) {
	var actualUsername, actualHostname string
	for _, env := range envVars {
		if env.Name == "KOPIA_OVERRIDE_USERNAME" {
			actualUsername = env.Value
		}
		if env.Name == "KOPIA_OVERRIDE_HOSTNAME" {
			actualHostname = env.Value
		}
	}

	if actualUsername != expectedUsername {
		t.Errorf("Expected username %s, got %s", expectedUsername, actualUsername)
	}
	if actualHostname != expectedHostname {
		t.Errorf("Expected hostname %s, got %s", expectedHostname, actualHostname)
	}
}

// TestManualConfigEdgeCases tests edge cases and error conditions
func TestManualConfigEdgeCases(t *testing.T) {
	tests := getEdgeCaseTestCases()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testManualConfigEdgeCase(t, tt)
		})
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

func testManualConfigEdgeCase(t *testing.T, tt edgeCaseTestCase) {
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
	verifyBasicEnvironmentExists(t, tt.name, tt.description, envVars)
}

func verifyBasicEnvironmentExists(t *testing.T, testName, description string, envVars []corev1.EnvVar) {
	if len(envVars) == 0 {
		t.Errorf("%s: No environment variables created (%s)", testName, description)
	}

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

	if !hasRepository || !hasPassword {
		t.Errorf("%s: Required variables missing (%s)", testName, description)
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
		return []string{"KOPIA_REPOSITORY", "KOPIA_PASSWORD", "KOPIA_FS_PATH"}
	default:
		return []string{"KOPIA_REPOSITORY", "KOPIA_PASSWORD"}
	}
}
