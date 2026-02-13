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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

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
			name: "latest retention only",
			retainPolicy: &volsyncv1alpha1.KopiaRetainPolicy{
				Latest: ptr.To[int32](10),
			},
			expectedEnvs: map[string]string{
				"KOPIA_RETAIN_LATEST": "10",
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
				Latest:  ptr.To[int32](100),
			},
			expectedEnvs: map[string]string{
				"KOPIA_RETAIN_HOURLY":  "24",
				"KOPIA_RETAIN_DAILY":   "30",
				"KOPIA_RETAIN_WEEKLY":  "8",
				"KOPIA_RETAIN_MONTHLY": "12",
				"KOPIA_RETAIN_YEARLY":  "5",
				"KOPIA_RETAIN_LATEST":  "100",
			},
		},
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

var _ = Describe("Backend Environment Variables", func() {
	DescribeTable("tests all 9 supported backends have proper environment variables",
		func(tt backendTestCase) {
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
			for _, reqVar := range tt.requiredEnvVars {
				found := false
				for _, env := range envVars {
					if env.Name == reqVar {
						found = true
						break
					}
				}
				Expect(found).To(BeTrue(), "Required variable %s not found", reqVar)
			}

			// Check optional variables
			for _, optVar := range tt.optionalEnvVars {
				found := false
				for _, env := range envVars {
					if env.Name == optVar {
						found = true
						// Verify it's properly configured as a secret reference
						Expect(env.ValueFrom).NotTo(BeNil(), "Variable %s should be a secret reference", optVar)
						Expect(env.ValueFrom.SecretKeyRef).NotTo(BeNil(), "Variable %s should be a secret reference", optVar)
						break
					}
				}
				Expect(found).To(BeTrue(), "Optional variable %s not found", optVar)
			}
		},
		Entry("S3 backend with all variables", getAWSS3BackendTestCase()),
		Entry("Azure Blob Storage with all variables", getAzureBackendTestCase()),
		Entry("Google Cloud Storage with all variables", getGoogleCloudTestCases()[0]),
		Entry("Google Drive with all variables", getGoogleCloudTestCases()[1]),
		Entry("Backblaze B2 with all variables", getBackblazeB2BackendTestCase()),
		Entry("WebDAV with all variables", getProtocolBackendTestCases()[0]),
		Entry("SFTP with all variables", getProtocolBackendTestCases()[1]),
		Entry("Rclone with all variables", getProtocolBackendTestCases()[2]),
		Entry("Filesystem with all variables", getFilesystemBackendTestCase()),
	)
})

var _ = Describe("Multi-Tenancy Features", func() {
	DescribeTable("tests Kopia's multi-tenancy features",
		func(tt multiTenancyTestCase) {
			// Test with ReplicationSource - for backward compatibility test with nil PVC name
			username := generateUsername(tt.customUsername, tt.replicationSource, tt.namespace)
			hostname := generateHostname(tt.customHostname, nil, tt.namespace, tt.replicationSource)

			Expect(username).To(Equal(tt.expectedUsername))
			Expect(hostname).To(Equal(tt.expectedHostname))

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
			var actualUsername, actualHostname string
			for _, env := range envVars {
				if env.Name == "KOPIA_OVERRIDE_USERNAME" {
					actualUsername = env.Value
				}
				if env.Name == "KOPIA_OVERRIDE_HOSTNAME" {
					actualHostname = env.Value
				}
			}

			Expect(actualUsername).To(Equal(tt.expectedUsername),
				"Environment variable KOPIA_OVERRIDE_USERNAME mismatch")
			Expect(actualHostname).To(Equal(tt.expectedHostname),
				"Environment variable KOPIA_OVERRIDE_HOSTNAME mismatch")
		},
		Entry("default username and hostname", getMultiTenancyTestCases()[0]),
		Entry("custom username only", getMultiTenancyTestCases()[1]),
		Entry("custom hostname only", getMultiTenancyTestCases()[2]),
		Entry("both custom username and hostname", getMultiTenancyTestCases()[3]),
		Entry("special characters in namespace/name", getMultiTenancyTestCases()[4]),
	)
})

var _ = Describe("Source Path Override Feature", func() {
	DescribeTable("tests the sourcePathOverride functionality",
		func(tt sourcePathOverrideTestCase) {
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
			found := false
			var actualValue string
			for _, env := range envVars {
				if env.Name == "KOPIA_SOURCE_PATH_OVERRIDE" {
					found = true
					actualValue = env.Value
					break
				}
			}

			Expect(found).To(Equal(tt.expectedEnvVarPresent),
				"Expected KOPIA_SOURCE_PATH_OVERRIDE present=%v, got %v", tt.expectedEnvVarPresent, found)

			if found {
				Expect(actualValue).To(Equal(tt.expectedValue),
					"Expected KOPIA_SOURCE_PATH_OVERRIDE value %s, got %s", tt.expectedValue, actualValue)
			}
		},
		Entry("no override specified", getSourcePathOverrideTestCases()[0]),
		Entry("standard path override", getSourcePathOverrideTestCases()[1]),
		Entry("nested path override", getSourcePathOverrideTestCases()[2]),
		Entry("root path override", getSourcePathOverrideTestCases()[3]),
	)
})

var _ = Describe("Kopia Actions Feature", func() {
	DescribeTable("tests the actions (hooks) functionality",
		func(tt actionsTestCase) {
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
			var actualBefore, actualAfter string
			for _, env := range envVars {
				if env.Name == "KOPIA_BEFORE_SNAPSHOT" {
					actualBefore = env.Value
				}
				if env.Name == "KOPIA_AFTER_SNAPSHOT" {
					actualAfter = env.Value
				}
			}

			Expect(actualBefore).To(Equal(tt.expectedBefore),
				"Expected KOPIA_BEFORE_SNAPSHOT %q, got %q", tt.expectedBefore, actualBefore)
			Expect(actualAfter).To(Equal(tt.expectedAfter),
				"Expected KOPIA_AFTER_SNAPSHOT %q, got %q", tt.expectedAfter, actualAfter)
		},
		Entry("no actions", getActionsTestCases()[0]),
		Entry("before action only", getActionsTestCases()[1]),
		Entry("after action only", getActionsTestCases()[2]),
		Entry("both before and after actions", getActionsTestCases()[3]),
	)
})

var _ = Describe("Kopia Compression And Parallelism", func() {
	DescribeTable("tests compression and parallelism settings",
		func(tt compressionParallelismTestCase) {
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
			var actualCompression, actualParallelism string
			for _, env := range envVars {
				if env.Name == "KOPIA_COMPRESSION" {
					actualCompression = env.Value
				}
				if env.Name == "KOPIA_PARALLELISM" {
					actualParallelism = env.Value
				}
			}

			Expect(actualCompression).To(Equal(tt.expectedCompression),
				"Expected KOPIA_COMPRESSION %q, got %q", tt.expectedCompression, actualCompression)
			Expect(actualParallelism).To(Equal(tt.expectedParallelism),
				"Expected KOPIA_PARALLELISM %q, got %q", tt.expectedParallelism, actualParallelism)
		},
		Entry("no compression or parallelism", getCompressionParallelismTestCases()[0]),
		Entry("zstd compression", getCompressionParallelismTestCases()[1]),
		Entry("gzip compression", getCompressionParallelismTestCases()[2]),
		Entry("s2 compression", getCompressionParallelismTestCases()[3]),
		Entry("no compression", getCompressionParallelismTestCases()[4]),
		Entry("parallelism set", getCompressionParallelismTestCases()[5]),
		Entry("both compression and parallelism", getCompressionParallelismTestCases()[6]),
	)
})

type compressionEnvTestCase struct {
	name                string
	compression         string
	expectedCompression string
}

var _ = Describe("Compression Environment Variable", func() {
	DescribeTable("tests that the compression environment variable is set correctly",
		func(tt compressionEnvTestCase) {
			// Create mover with minimal setup
			mover := &Mover{
				compression: tt.compression,
				isSource:    true,
			}

			// Get environment variables
			envVars := mover.addSourceEnvVars([]corev1.EnvVar{})

			// Check if KOPIA_COMPRESSION is set correctly
			found := false
			for _, env := range envVars {
				if env.Name == "KOPIA_COMPRESSION" {
					found = true
					Expect(env.Value).To(Equal(tt.expectedCompression),
						"Expected KOPIA_COMPRESSION=%q, got %q", tt.expectedCompression, env.Value)
				}
			}

			if tt.expectedCompression != "" {
				Expect(found).To(BeTrue(),
					"KOPIA_COMPRESSION environment variable not found, expected %q", tt.expectedCompression)
			} else {
				Expect(found).To(BeFalse(),
					"KOPIA_COMPRESSION environment variable found when not expected")
			}
		},
		Entry("valid zstd compression sets env var", compressionEnvTestCase{
			name:                "valid zstd compression sets env var",
			compression:         "zstd",
			expectedCompression: "zstd",
		}),
		Entry("valid gzip compression sets env var", compressionEnvTestCase{
			name:                "valid gzip compression sets env var",
			compression:         "gzip-best-speed",
			expectedCompression: "gzip-best-speed",
		}),
		Entry("empty compression does not set env var", compressionEnvTestCase{
			name:                "empty compression does not set env var",
			compression:         "",
			expectedCompression: "",
		}),
	)
})

// Compression validation tests have been removed.
// Compression values are passed directly to Kopia without validation.
// Kopia handles validation of the compression algorithm and provides clear error messages.

var _ = Describe("Kopia Retention Policy", func() {
	DescribeTable("tests retention policy settings",
		func(tt retentionPolicyTestCase) {
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
			actualEnvs := make(map[string]string)
			for _, env := range envVars {
				if strings.HasPrefix(env.Name, "KOPIA_RETAIN_") {
					actualEnvs[env.Name] = env.Value
				}
			}

			// Verify expected environment variables
			for key, expectedValue := range tt.expectedEnvs {
				actualValue, ok := actualEnvs[key]
				Expect(ok).To(BeTrue(), "Expected environment variable %s not found", key)
				Expect(actualValue).To(Equal(expectedValue), "Expected %s=%s, got %s", key, expectedValue, actualValue)
			}

			// Verify no unexpected retention variables
			for key := range actualEnvs {
				_, expected := tt.expectedEnvs[key]
				Expect(expected).To(BeTrue(), "Unexpected environment variable %s", key)
			}
		},
		Entry("no retention policy", getRetentionPolicyTestCases()[0]),
		Entry("hourly retention only", getRetentionPolicyTestCases()[1]),
		Entry("latest retention only", getRetentionPolicyTestCases()[2]),
		Entry("complete retention policy", getRetentionPolicyTestCases()[3]),
	)
})

var _ = Describe("Kopia Destination Features", func() {
	DescribeTable("tests destination-specific features",
		func(tt destinationFeaturesTestCase) {
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
			actualEnvs := make(map[string]string)
			for _, env := range envVars {
				if env.Name == "KOPIA_RESTORE_AS_OF" || env.Name == "KOPIA_SHALLOW" || env.Name == "KOPIA_PREVIOUS" {
					actualEnvs[env.Name] = env.Value
				}
			}

			// Verify expected environment variables
			for key, expectedValue := range tt.expectedEnvs {
				actualValue, ok := actualEnvs[key]
				Expect(ok).To(BeTrue(), "Expected environment variable %s not found", key)
				Expect(actualValue).To(Equal(expectedValue), "Expected %s=%s, got %s", key, expectedValue, actualValue)
			}

			// Verify no unexpected environment variables
			for key := range actualEnvs {
				_, expected := tt.expectedEnvs[key]
				Expect(expected).To(BeTrue(), "Unexpected environment variable %s", key)
			}
		},
		Entry("no restore options", getDestinationFeaturesTestCases()[0]),
		Entry("restore as of timestamp", getDestinationFeaturesTestCases()[1]),
		Entry("shallow restore", getDestinationFeaturesTestCases()[2]),
		Entry("both restore as of and shallow", getDestinationFeaturesTestCases()[3]),
		Entry("previous restore", getDestinationFeaturesTestCases()[4]),
		Entry("previous with restore as of", getDestinationFeaturesTestCases()[5]),
	)
})
