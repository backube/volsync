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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

// TestAllBackendEnvironmentVariables tests that all 9 supported backends have proper environment variables
func TestAllBackendEnvironmentVariables(t *testing.T) {
	tests := getBackendTestCases()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testBackendEnvironmentVariables(t, tt)
		})
	}
}

type backendTestCase struct {
	name            string
	secretData      map[string][]byte
	requiredEnvVars []string
	optionalEnvVars []string
}

func getBackendTestCases() []backendTestCase {
	var cases []backendTestCase

	cases = append(cases, getCloudBackendTestCases()...)
	cases = append(cases, getProtocolBackendTestCases()...)
	cases = append(cases, getFilesystemBackendTestCase())

	return cases
}

func getCloudBackendTestCases() []backendTestCase {
	var cases []backendTestCase

	cases = append(cases, getAWSS3BackendTestCase())
	cases = append(cases, getAzureBackendTestCase())
	cases = append(cases, getGoogleCloudTestCases()...)
	cases = append(cases, getBackblazeB2BackendTestCase())

	return cases
}

func getAWSS3BackendTestCase() backendTestCase {
	return backendTestCase{
		name: "S3 backend with all variables",
		secretData: map[string][]byte{
			"KOPIA_REPOSITORY":      []byte("s3://my-bucket/path"),
			"KOPIA_PASSWORD":        []byte("password"),
			"AWS_ACCESS_KEY_ID":     []byte("AKIAIOSFODNN7EXAMPLE"),
			"AWS_SECRET_ACCESS_KEY": []byte("wJalrXUtnFEMI/K7MDENG"),
			"AWS_SESSION_TOKEN":     []byte("session-token"),
			"AWS_DEFAULT_REGION":    []byte("us-west-2"),
			"AWS_PROFILE":           []byte("default"),
			"KOPIA_S3_BUCKET":       []byte("my-bucket"),
			"KOPIA_S3_ENDPOINT":     []byte("s3.amazonaws.com"),
			"KOPIA_S3_DISABLE_TLS":  []byte("false"),
		},
		requiredEnvVars: []string{"KOPIA_REPOSITORY", "KOPIA_PASSWORD"},
		optionalEnvVars: []string{
			"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN",
			"AWS_DEFAULT_REGION", "AWS_PROFILE", "KOPIA_S3_BUCKET",
			"KOPIA_S3_ENDPOINT", "AWS_S3_ENDPOINT", "KOPIA_S3_DISABLE_TLS", "AWS_S3_DISABLE_TLS",
			"AWS_REGION",
		},
	}
}

func getAzureBackendTestCase() backendTestCase {
	return backendTestCase{
		name: "Azure Blob Storage with all variables",
		secretData: map[string][]byte{
			"KOPIA_REPOSITORY":            []byte("azure://container/path"),
			"KOPIA_PASSWORD":              []byte("password"),
			"AZURE_ACCOUNT_NAME":          []byte("myaccount"),
			"AZURE_ACCOUNT_KEY":           []byte("account-key"),
			"AZURE_ACCOUNT_SAS":           []byte("sas-token"),
			"AZURE_ENDPOINT_SUFFIX":       []byte("core.windows.net"),
			"KOPIA_AZURE_CONTAINER":       []byte("container"),
			"KOPIA_AZURE_STORAGE_ACCOUNT": []byte("myaccount"),
			"KOPIA_AZURE_STORAGE_KEY":     []byte("storage-key"),
		},
		requiredEnvVars: []string{"KOPIA_REPOSITORY", "KOPIA_PASSWORD"},
		optionalEnvVars: []string{
			"AZURE_ACCOUNT_NAME", "AZURE_ACCOUNT_KEY", "AZURE_ACCOUNT_SAS",
			"AZURE_ENDPOINT_SUFFIX", "KOPIA_AZURE_CONTAINER",
			"KOPIA_AZURE_STORAGE_ACCOUNT", "AZURE_STORAGE_ACCOUNT",
			"KOPIA_AZURE_STORAGE_KEY", "AZURE_STORAGE_KEY",
			"AZURE_STORAGE_SAS_TOKEN",
		},
	}
}

func getGoogleCloudTestCases() []backendTestCase {
	return []backendTestCase{
		{
			name: "Google Cloud Storage with all variables",
			secretData: map[string][]byte{
				"KOPIA_REPOSITORY":               []byte("gcs://my-bucket/path"),
				"KOPIA_PASSWORD":                 []byte("password"),
				"GOOGLE_PROJECT_ID":              []byte("my-project"),
				"KOPIA_GCS_BUCKET":               []byte("my-bucket"),
				"GOOGLE_APPLICATION_CREDENTIALS": []byte(`{"type":"service_account"}`),
			},
			requiredEnvVars: []string{"KOPIA_REPOSITORY", "KOPIA_PASSWORD"},
			optionalEnvVars: []string{
				"GOOGLE_PROJECT_ID", "KOPIA_GCS_BUCKET",
				"GOOGLE_APPLICATION_CREDENTIALS",
			},
		},
		{
			name: "Google Drive with all variables",
			secretData: map[string][]byte{
				"KOPIA_REPOSITORY":         []byte("gdrive://folder-id"),
				"KOPIA_PASSWORD":           []byte("password"),
				"GOOGLE_DRIVE_FOLDER_ID":   []byte("folder-id"),
				"GOOGLE_DRIVE_CREDENTIALS": []byte(`{"type":"oauth2"}`),
			},
			requiredEnvVars: []string{"KOPIA_REPOSITORY", "KOPIA_PASSWORD"},
			optionalEnvVars: []string{"GOOGLE_DRIVE_FOLDER_ID", "GOOGLE_DRIVE_CREDENTIALS"},
		},
	}
}

func getBackblazeB2BackendTestCase() backendTestCase {
	return backendTestCase{
		name: "Backblaze B2 with all variables",
		secretData: map[string][]byte{
			"KOPIA_REPOSITORY":   []byte("b2://my-bucket/path"),
			"KOPIA_PASSWORD":     []byte("password"),
			"B2_ACCOUNT_ID":      []byte("account-id"),
			"B2_APPLICATION_KEY": []byte("app-key"),
			"KOPIA_B2_BUCKET":    []byte("my-bucket"),
		},
		requiredEnvVars: []string{"KOPIA_REPOSITORY", "KOPIA_PASSWORD"},
		optionalEnvVars: []string{"B2_ACCOUNT_ID", "B2_APPLICATION_KEY", "KOPIA_B2_BUCKET"},
	}
}

func getProtocolBackendTestCases() []backendTestCase {
	return []backendTestCase{
		{
			name: "WebDAV with all variables",
			secretData: map[string][]byte{
				"KOPIA_REPOSITORY": []byte("webdav://server.com/path"),
				"KOPIA_PASSWORD":   []byte("password"),
				"WEBDAV_URL":       []byte("https://server.com/webdav"),
				"WEBDAV_USERNAME":  []byte("user"),
				"WEBDAV_PASSWORD":  []byte("webdav-pass"),
			},
			requiredEnvVars: []string{"KOPIA_REPOSITORY", "KOPIA_PASSWORD"},
			optionalEnvVars: []string{"WEBDAV_URL", "WEBDAV_USERNAME", "WEBDAV_PASSWORD"},
		},
		{
			name: "SFTP with all variables",
			secretData: map[string][]byte{
				"KOPIA_REPOSITORY": []byte("sftp://server.com/path"),
				"KOPIA_PASSWORD":   []byte("password"),
				"SFTP_HOST":        []byte("server.com"),
				"SFTP_PORT":        []byte("22"),
				"SFTP_USERNAME":    []byte("user"),
				"SFTP_PASSWORD":    []byte("sftp-pass"),
				"SFTP_PATH":        []byte("/backup"),
				"SFTP_KEY_FILE":    []byte("ssh-private-key"),
			},
			requiredEnvVars: []string{"KOPIA_REPOSITORY", "KOPIA_PASSWORD"},
			optionalEnvVars: []string{
				"SFTP_HOST", "SFTP_PORT", "SFTP_USERNAME",
				"SFTP_PASSWORD", "SFTP_PATH", "SFTP_KEY_FILE",
			},
		},
		{
			name: "Rclone with all variables",
			secretData: map[string][]byte{
				"KOPIA_REPOSITORY":   []byte("rclone://remote:path"),
				"KOPIA_PASSWORD":     []byte("password"),
				"RCLONE_REMOTE_PATH": []byte("remote:path"),
				"RCLONE_EXE":         []byte("/usr/bin/rclone"),
				"RCLONE_CONFIG":      []byte("[remote]\ntype = s3\n..."),
			},
			requiredEnvVars: []string{"KOPIA_REPOSITORY", "KOPIA_PASSWORD"},
			optionalEnvVars: []string{"RCLONE_REMOTE_PATH", "RCLONE_EXE", "RCLONE_CONFIG"},
		},
	}
}

func getFilesystemBackendTestCase() backendTestCase {
	return backendTestCase{
		name: "Filesystem with all variables",
		secretData: map[string][]byte{
			"KOPIA_REPOSITORY": []byte("filesystem:///mnt/backup"),
			"KOPIA_PASSWORD":   []byte("password"),
		},
		requiredEnvVars: []string{"KOPIA_REPOSITORY", "KOPIA_PASSWORD"},
		optionalEnvVars: []string{},
	}
}

func testBackendEnvironmentVariables(t *testing.T, tt backendTestCase) {
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

	// Check required variables
	verifyRequiredVariables(t, tt.name, envVars, tt.requiredEnvVars)

	// Check optional variables
	verifyOptionalVariables(t, tt.name, envVars, tt.optionalEnvVars)
}

func verifyRequiredVariables(t *testing.T, testName string, envVars []corev1.EnvVar, requiredVars []string) {
	for _, reqVar := range requiredVars {
		found := false
		for _, env := range envVars {
			if env.Name == reqVar {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s: Required variable %s not found", testName, reqVar)
		}
	}
}

func verifyOptionalVariables(t *testing.T, testName string, envVars []corev1.EnvVar, optionalVars []string) {
	for _, optVar := range optionalVars {
		found := false
		for _, env := range envVars {
			if env.Name == optVar {
				found = true
				// Verify it's properly configured as a secret reference
				if env.ValueFrom == nil || env.ValueFrom.SecretKeyRef == nil {
					t.Errorf("%s: Variable %s should be a secret reference", testName, optVar)
				}
				break
			}
		}
		if !found {
			t.Errorf("%s: Optional variable %s not found", testName, optVar)
		}
	}
}

// TestMultiTenancyFeatures tests Kopia's multi-tenancy features
func TestMultiTenancyFeatures(t *testing.T) {
	tests := getMultiTenancyTestCases()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testMultiTenancyFeature(t, tt)
		})
	}
}

type multiTenancyTestCase struct {
	name              string
	namespace         string
	replicationSource string
	customUsername    *string
	customHostname    *string
	expectedUsername  string
	expectedHostname  string
}

func getMultiTenancyTestCases() []multiTenancyTestCase {
	return []multiTenancyTestCase{
		{
			name:              "default username and hostname",
			namespace:         "production",
			replicationSource: "app-backup",
			customUsername:    nil,
			customHostname:    nil,
			expectedUsername:  "app-backup", // Object name only
			expectedHostname:  "production",
		},
		{
			name:              "custom username only",
			namespace:         "staging",
			replicationSource: "database",
			customUsername:    ptr.To("custom-user"),
			customHostname:    nil,
			expectedUsername:  "custom-user",
			expectedHostname:  "staging",
		},
		{
			name:              "custom hostname only",
			namespace:         "dev",
			replicationSource: "logs",
			customUsername:    nil,
			customHostname:    ptr.To("dev-cluster"),
			expectedUsername:  "logs", // Object name only
			expectedHostname:  "dev-cluster",
		},
		{
			name:              "both custom username and hostname",
			namespace:         "test",
			replicationSource: "config",
			customUsername:    ptr.To("backup-service"),
			customHostname:    ptr.To("cluster-west-1"),
			expectedUsername:  "backup-service",
			expectedHostname:  "cluster-west-1",
		},
		{
			name:              "special characters in namespace/name",
			namespace:         "my-namespace",
			replicationSource: "app_backup_job",
			customUsername:    nil,
			customHostname:    nil,
			expectedUsername:  "app_backup_job", // Object name only
			expectedHostname:  "my-namespace",   // namespace-first logic uses just namespace when no PVC
		},
	}
}

func testMultiTenancyFeature(t *testing.T, tt multiTenancyTestCase) {
	// Test with ReplicationSource - for backward compatibility test with nil PVC name
	username := generateUsername(tt.customUsername, tt.replicationSource, tt.namespace)
	hostname := generateHostname(tt.customHostname, nil, tt.namespace, tt.replicationSource)

	if username != tt.expectedUsername {
		t.Errorf("Expected username %s, got %s", tt.expectedUsername, username)
	}
	if hostname != tt.expectedHostname {
		t.Errorf("Expected hostname %s, got %s", tt.expectedHostname, hostname)
	}

	// Verify the generated values work in environment variables
	owner := &volsyncv1alpha1.ReplicationSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tt.replicationSource,
			Namespace: tt.namespace,
		},
	}

	mover := &Mover{
		username: username,
		hostname: hostname,
		owner:    owner,
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-secret",
		},
		Data: map[string][]byte{
			"KOPIA_REPOSITORY": []byte("s3://bucket/path"),
			"KOPIA_PASSWORD":   []byte("password"),
		},
	}

	envVars := mover.buildEnvironmentVariables(secret)

	// Check override environment variables
	verifyOverrideEnvironmentVariables(t, tt.expectedUsername, tt.expectedHostname, envVars)
}

func verifyOverrideEnvironmentVariables(
	t *testing.T,
	expectedUsername, expectedHostname string,
	envVars []corev1.EnvVar,
) {
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
		t.Errorf("Environment variable KOPIA_OVERRIDE_USERNAME: expected %s, got %s",
			expectedUsername, actualUsername)
	}
	if actualHostname != expectedHostname {
		t.Errorf("Environment variable KOPIA_OVERRIDE_HOSTNAME: expected %s, got %s",
			expectedHostname, actualHostname)
	}
}

// TestSourcePathOverrideFeature tests the sourcePathOverride functionality
func TestSourcePathOverrideFeature(t *testing.T) {
	tests := getSourcePathOverrideTestCases()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testSourcePathOverride(t, tt)
		})
	}
}

type sourcePathOverrideTestCase struct {
	name                  string
	sourcePathOverride    *string
	expectedEnvVarPresent bool
	expectedValue         string
}

func getSourcePathOverrideTestCases() []sourcePathOverrideTestCase {
	return []sourcePathOverrideTestCase{
		{
			name:                  "no override specified",
			sourcePathOverride:    nil,
			expectedEnvVarPresent: false,
		},
		{
			name:                  "standard path override",
			sourcePathOverride:    ptr.To("/var/lib/postgresql/data"),
			expectedEnvVarPresent: true,
			expectedValue:         "/var/lib/postgresql/data",
		},
		{
			name:                  "nested path override",
			sourcePathOverride:    ptr.To("/opt/application/data/persistent"),
			expectedEnvVarPresent: true,
			expectedValue:         "/opt/application/data/persistent",
		},
		{
			name:                  "root path override",
			sourcePathOverride:    ptr.To("/"),
			expectedEnvVarPresent: true,
			expectedValue:         "/",
		},
	}
}

func testSourcePathOverride(t *testing.T, tt sourcePathOverrideTestCase) {
	owner := &volsyncv1alpha1.ReplicationSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rs",
			Namespace: "test-namespace",
		},
	}

	mover := &Mover{
		username:           "test-user",
		hostname:           "test-host",
		owner:              owner,
		isSource:           true,
		sourcePathOverride: tt.sourcePathOverride,
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-secret",
		},
		Data: map[string][]byte{
			"KOPIA_REPOSITORY": []byte("s3://bucket/path"),
			"KOPIA_PASSWORD":   []byte("password"),
		},
	}

	envVars := mover.buildEnvironmentVariables(secret)

	// Check for KOPIA_SOURCE_PATH_OVERRIDE
	verifySourcePathOverrideVariable(t, tt, envVars)
}

func verifySourcePathOverrideVariable(t *testing.T, tt sourcePathOverrideTestCase, envVars []corev1.EnvVar) {
	found := false
	var actualValue string
	for _, env := range envVars {
		if env.Name == "KOPIA_SOURCE_PATH_OVERRIDE" {
			found = true
			actualValue = env.Value
			break
		}
	}

	if found != tt.expectedEnvVarPresent {
		t.Errorf("Expected KOPIA_SOURCE_PATH_OVERRIDE present=%v, got %v",
			tt.expectedEnvVarPresent, found)
	}

	if found && actualValue != tt.expectedValue {
		t.Errorf("Expected KOPIA_SOURCE_PATH_OVERRIDE value %s, got %s",
			tt.expectedValue, actualValue)
	}
}

// TestKopiaActionsFeature tests the actions (hooks) functionality
func TestKopiaActionsFeature(t *testing.T) {
	tests := getActionsTestCases()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testKopiaActions(t, tt)
		})
	}
}

type actionsTestCase struct {
	name           string
	actions        *volsyncv1alpha1.KopiaActions
	expectedBefore string
	expectedAfter  string
}

func getActionsTestCases() []actionsTestCase {
	return []actionsTestCase{
		{
			name:    "no actions",
			actions: nil,
		},
		{
			name: "before action only",
			actions: &volsyncv1alpha1.KopiaActions{
				BeforeSnapshot: "sync && echo 3 > /proc/sys/vm/drop_caches",
			},
			expectedBefore: "sync && echo 3 > /proc/sys/vm/drop_caches",
		},
		{
			name: "after action only",
			actions: &volsyncv1alpha1.KopiaActions{
				AfterSnapshot: "echo 'Backup completed'",
			},
			expectedAfter: "echo 'Backup completed'",
		},
		{
			name: "both before and after actions",
			actions: &volsyncv1alpha1.KopiaActions{
				BeforeSnapshot: "mysqldump --all-databases > /data/backup.sql",
				AfterSnapshot:  "rm -f /data/backup.sql",
			},
			expectedBefore: "mysqldump --all-databases > /data/backup.sql",
			expectedAfter:  "rm -f /data/backup.sql",
		},
	}
}

func testKopiaActions(t *testing.T, tt actionsTestCase) {
	owner := &volsyncv1alpha1.ReplicationSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rs",
			Namespace: "test-namespace",
		},
	}

	mover := &Mover{
		username: "test-user",
		hostname: "test-host",
		owner:    owner,
		isSource: true,
		actions:  tt.actions,
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-secret",
		},
		Data: map[string][]byte{
			"KOPIA_REPOSITORY": []byte("s3://bucket/path"),
			"KOPIA_PASSWORD":   []byte("password"),
		},
	}

	envVars := mover.buildEnvironmentVariables(secret)

	// Check for action environment variables
	verifyActionEnvironmentVariables(t, tt.expectedBefore, tt.expectedAfter, envVars)
}

func verifyActionEnvironmentVariables(t *testing.T, expectedBefore, expectedAfter string, envVars []corev1.EnvVar) {
	var actualBefore, actualAfter string
	for _, env := range envVars {
		if env.Name == "KOPIA_BEFORE_SNAPSHOT" {
			actualBefore = env.Value
		}
		if env.Name == "KOPIA_AFTER_SNAPSHOT" {
			actualAfter = env.Value
		}
	}

	if actualBefore != expectedBefore {
		t.Errorf("Expected KOPIA_BEFORE_SNAPSHOT %q, got %q",
			expectedBefore, actualBefore)
	}
	if actualAfter != expectedAfter {
		t.Errorf("Expected KOPIA_AFTER_SNAPSHOT %q, got %q",
			expectedAfter, actualAfter)
	}
}

// TestKopiaCompressionAndParallelism tests compression and parallelism settings
func TestKopiaCompressionAndParallelism(t *testing.T) {
	tests := getCompressionParallelismTestCases()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testCompressionAndParallelism(t, tt)
		})
	}
}

type compressionParallelismTestCase struct {
	name                string
	compression         string
	parallelism         *int32
	expectedCompression string
	expectedParallelism string
}

func getCompressionParallelismTestCases() []compressionParallelismTestCase {
	return []compressionParallelismTestCase{
		{
			name:        "no compression or parallelism",
			compression: "",
			parallelism: nil,
		},
		{
			name:                "zstd compression",
			compression:         "zstd",
			expectedCompression: "zstd",
		},
		{
			name:                "gzip compression",
			compression:         "gzip",
			expectedCompression: "gzip",
		},
		{
			name:                "s2 compression",
			compression:         "s2",
			expectedCompression: "s2",
		},
		{
			name:                "no compression",
			compression:         "none",
			expectedCompression: "none",
		},
		{
			name:                "parallelism set",
			parallelism:         ptr.To[int32](4),
			expectedParallelism: "4",
		},
		{
			name:                "both compression and parallelism",
			compression:         "zstd",
			parallelism:         ptr.To[int32](8),
			expectedCompression: "zstd",
			expectedParallelism: "8",
		},
	}
}

func testCompressionAndParallelism(t *testing.T, tt compressionParallelismTestCase) {
	owner := &volsyncv1alpha1.ReplicationSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rs",
			Namespace: "test-namespace",
		},
	}

	mover := &Mover{
		username:    "test-user",
		hostname:    "test-host",
		owner:       owner,
		isSource:    true,
		compression: tt.compression,
		parallelism: tt.parallelism,
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-secret",
		},
		Data: map[string][]byte{
			"KOPIA_REPOSITORY": []byte("s3://bucket/path"),
			"KOPIA_PASSWORD":   []byte("password"),
		},
	}

	envVars := mover.buildEnvironmentVariables(secret)

	// Check for compression and parallelism environment variables
	verifyCompressionParallelismVariables(t, tt.expectedCompression, tt.expectedParallelism, envVars)
}

func verifyCompressionParallelismVariables(
	t *testing.T,
	expectedCompression, expectedParallelism string,
	envVars []corev1.EnvVar,
) {
	var actualCompression, actualParallelism string
	for _, env := range envVars {
		if env.Name == "KOPIA_COMPRESSION" {
			actualCompression = env.Value
		}
		if env.Name == "KOPIA_PARALLELISM" {
			actualParallelism = env.Value
		}
	}

	if actualCompression != expectedCompression {
		t.Errorf("Expected KOPIA_COMPRESSION %q, got %q",
			expectedCompression, actualCompression)
	}
	if actualParallelism != expectedParallelism {
		t.Errorf("Expected KOPIA_PARALLELISM %q, got %q",
			expectedParallelism, actualParallelism)
	}
}

// TestKopiaRetentionPolicy tests retention policy settings
func TestKopiaRetentionPolicy(t *testing.T) {
	tests := getRetentionPolicyTestCases()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testRetentionPolicy(t, tt)
		})
	}
}

type retentionPolicyTestCase struct {
	name         string
	retainPolicy *volsyncv1alpha1.KopiaRetainPolicy
	expectedEnvs map[string]string
}

func getRetentionPolicyTestCases() []retentionPolicyTestCase {
	return []retentionPolicyTestCase{
		{
			name:         "no retention policy",
			retainPolicy: nil,
			expectedEnvs: map[string]string{},
		},
		{
			name: "hourly retention only",
			retainPolicy: &volsyncv1alpha1.KopiaRetainPolicy{
				Hourly: ptr.To[int32](24),
			},
			expectedEnvs: map[string]string{
				"KOPIA_RETAIN_HOURLY": "24",
			},
		},
		{
			name: "complete retention policy",
			retainPolicy: &volsyncv1alpha1.KopiaRetainPolicy{
				Hourly:  ptr.To[int32](24),
				Daily:   ptr.To[int32](30),
				Weekly:  ptr.To[int32](8),
				Monthly: ptr.To[int32](12),
				Yearly:  ptr.To[int32](5),
			},
			expectedEnvs: map[string]string{
				"KOPIA_RETAIN_HOURLY":  "24",
				"KOPIA_RETAIN_DAILY":   "30",
				"KOPIA_RETAIN_WEEKLY":  "8",
				"KOPIA_RETAIN_MONTHLY": "12",
				"KOPIA_RETAIN_YEARLY":  "5",
			},
		},
	}
}

func testRetentionPolicy(t *testing.T, tt retentionPolicyTestCase) {
	owner := &volsyncv1alpha1.ReplicationSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rs",
			Namespace: "test-namespace",
		},
	}

	mover := &Mover{
		username:     "test-user",
		hostname:     "test-host",
		owner:        owner,
		isSource:     true,
		retainPolicy: tt.retainPolicy,
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-secret",
		},
		Data: map[string][]byte{
			"KOPIA_REPOSITORY": []byte("s3://bucket/path"),
			"KOPIA_PASSWORD":   []byte("password"),
		},
	}

	envVars := mover.buildEnvironmentVariables(secret)

	// Check for retention policy environment variables
	verifyRetentionPolicyVariables(t, tt.expectedEnvs, envVars)
}

func verifyRetentionPolicyVariables(t *testing.T, expectedEnvs map[string]string, envVars []corev1.EnvVar) {
	actualEnvs := make(map[string]string)
	for _, env := range envVars {
		if strings.HasPrefix(env.Name, "KOPIA_RETAIN_") {
			actualEnvs[env.Name] = env.Value
		}
	}

	// Verify expected environment variables
	for key, expectedValue := range expectedEnvs {
		if actualValue, ok := actualEnvs[key]; !ok {
			t.Errorf("Expected environment variable %s not found", key)
		} else if actualValue != expectedValue {
			t.Errorf("Expected %s=%s, got %s", key, expectedValue, actualValue)
		}
	}

	// Verify no unexpected retention variables
	for key := range actualEnvs {
		if _, expected := expectedEnvs[key]; !expected {
			t.Errorf("Unexpected environment variable %s", key)
		}
	}
}

// TestKopiaDestinationFeatures tests destination-specific features
func TestKopiaDestinationFeatures(t *testing.T) {
	tests := getDestinationFeaturesTestCases()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testDestinationFeatures(t, tt)
		})
	}
}

type destinationFeaturesTestCase struct {
	name         string
	restoreAsOf  *string
	shallow      *int32
	previous     *int32
	expectedEnvs map[string]string
}

func getDestinationFeaturesTestCases() []destinationFeaturesTestCase {
	return []destinationFeaturesTestCase{
		{
			name:         "no restore options",
			restoreAsOf:  nil,
			shallow:      nil,
			expectedEnvs: map[string]string{},
		},
		{
			name:        "restore as of timestamp",
			restoreAsOf: ptr.To("2024-01-15T18:30:00Z"),
			expectedEnvs: map[string]string{
				"KOPIA_RESTORE_AS_OF": "2024-01-15T18:30:00Z",
			},
		},
		{
			name:    "shallow restore",
			shallow: ptr.To[int32](5),
			expectedEnvs: map[string]string{
				"KOPIA_SHALLOW": "5",
			},
		},
		{
			name:        "both restore as of and shallow",
			restoreAsOf: ptr.To("2024-01-15T18:30:00Z"),
			shallow:     ptr.To[int32](3),
			expectedEnvs: map[string]string{
				"KOPIA_RESTORE_AS_OF": "2024-01-15T18:30:00Z",
				"KOPIA_SHALLOW":       "3",
			},
		},
		{
			name:     "previous restore",
			previous: ptr.To[int32](1),
			expectedEnvs: map[string]string{
				"KOPIA_PREVIOUS": "1",
			},
		},
		{
			name:        "previous with restore as of",
			restoreAsOf: ptr.To("2024-01-15T18:30:00Z"),
			previous:    ptr.To[int32](2),
			expectedEnvs: map[string]string{
				"KOPIA_RESTORE_AS_OF": "2024-01-15T18:30:00Z",
				"KOPIA_PREVIOUS":      "2",
			},
		},
	}
}

func testDestinationFeatures(t *testing.T, tt destinationFeaturesTestCase) {
	owner := &volsyncv1alpha1.ReplicationDestination{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rd",
			Namespace: "test-namespace",
		},
	}

	mover := &Mover{
		username:    "test-user",
		hostname:    "test-host",
		owner:       owner,
		isSource:    false, // Destination
		restoreAsOf: tt.restoreAsOf,
		shallow:     tt.shallow,
		previous:    tt.previous,
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-secret",
		},
		Data: map[string][]byte{
			"KOPIA_REPOSITORY": []byte("s3://bucket/path"),
			"KOPIA_PASSWORD":   []byte("password"),
		},
	}

	envVars := mover.buildEnvironmentVariables(secret)

	// Check for destination-specific environment variables
	verifyDestinationFeatureVariables(t, tt.expectedEnvs, envVars)
}

func verifyDestinationFeatureVariables(t *testing.T, expectedEnvs map[string]string, envVars []corev1.EnvVar) {
	actualEnvs := make(map[string]string)
	for _, env := range envVars {
		if env.Name == "KOPIA_RESTORE_AS_OF" || env.Name == "KOPIA_SHALLOW" || env.Name == "KOPIA_PREVIOUS" {
			actualEnvs[env.Name] = env.Value
		}
	}

	// Verify expected environment variables
	for key, expectedValue := range expectedEnvs {
		if actualValue, ok := actualEnvs[key]; !ok {
			t.Errorf("Expected environment variable %s not found", key)
		} else if actualValue != expectedValue {
			t.Errorf("Expected %s=%s, got %s", key, expectedValue, actualValue)
		}
	}

	// Verify no unexpected environment variables
	for key := range actualEnvs {
		if _, expected := expectedEnvs[key]; !expected {
			t.Errorf("Unexpected environment variable %s", key)
		}
	}
}
