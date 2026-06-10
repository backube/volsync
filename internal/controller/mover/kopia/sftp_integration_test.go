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
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

var _ = Describe("SFTP Password Configuration", func() {
	type sftpPasswordConfigTestCase struct {
		secretData     map[string][]byte
		expectPassword bool
		expectKeyFile  bool
	}

	DescribeTable("properly configures SFTP password authentication",
		func(tc sftpPasswordConfigTestCase) {
			// Create a mock owner
			owner := &volsyncv1alpha1.ReplicationSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-source",
					Namespace: "test-namespace",
				},
			}

			mover := &Mover{
				logger:   logr.Discard(),
				owner:    owner,
				username: "test-user",
				hostname: "test-host",
			}

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-secret",
				},
				Data: tc.secretData,
			}

			// Build environment variables
			envVars := mover.buildEnvironmentVariables(secret)

			// Check SFTP_PASSWORD
			// Note: Environment variables are always created with optional: true,
			// regardless of whether they exist in the secret data
			foundPassword := false
			for _, env := range envVars {
				if env.Name == "SFTP_PASSWORD" {
					foundPassword = true
					// Verify it's from secret
					Expect(env.ValueFrom).NotTo(BeNil(), "SFTP_PASSWORD should be from secret")
					Expect(env.ValueFrom.SecretKeyRef).NotTo(BeNil(), "SFTP_PASSWORD should be from secret")
					Expect(env.ValueFrom.SecretKeyRef.Name).To(Equal(secret.Name),
						"SFTP_PASSWORD should reference secret %s", secret.Name)
					Expect(env.ValueFrom.SecretKeyRef.Key).To(Equal("SFTP_PASSWORD"),
						"SFTP_PASSWORD should reference correct key")
					// Should be optional
					Expect(*env.ValueFrom.SecretKeyRef.Optional).To(BeTrue(),
						"SFTP_PASSWORD should be optional")
					break
				}
			}

			// SFTP_PASSWORD env var is always present, but it only has a value if it exists in the secret
			Expect(foundPassword).To(BeTrue(), "SFTP_PASSWORD environment variable should always be present")

			// Check SFTP_KEY_FILE handling via credentials configuration
			podSpec := &corev1.PodSpec{
				Containers: []corev1.Container{{
					Name:  "kopia",
					Image: "test-image",
					Env:   []corev1.EnvVar{},
				}},
				Volumes: []corev1.Volume{},
			}

			mover.configureCredentials(podSpec, secret)

			if tc.expectKeyFile {
				// Check that SSH key file is mounted
				foundKeyMount := false
				for _, mount := range podSpec.Containers[0].VolumeMounts {
					if mount.MountPath == "/credentials" {
						foundKeyMount = true
						break
					}
				}
				Expect(foundKeyMount).To(BeTrue(), "Expected SSH key mount not found")

				// Check the volume contains the SSH key
				Expect(podSpec.Volumes).NotTo(BeEmpty(), "Expected volume for SSH key not found")
				volume := podSpec.Volumes[0]
				foundKey := false
				for _, item := range volume.Secret.Items {
					if item.Path == "sftp_key" {
						foundKey = true
						// Verify permissions
						Expect(item.Mode).NotTo(BeNil(), "SSH key mode should be set")
						Expect(*item.Mode).To(Equal(int32(0600)), "SSH key should have mode 0600")
						break
					}
				}
				Expect(foundKey).To(BeTrue(), "SSH key file not found in volume")
			}
		},
		Entry("SFTP with password only", sftpPasswordConfigTestCase{
			secretData: map[string][]byte{
				"SFTP_HOST":     []byte("sftp.example.com"),
				"SFTP_USERNAME": []byte("user"),
				"SFTP_PASSWORD": []byte("secret-pass"),
				"SFTP_PATH":     []byte("/backup"),
			},
			expectPassword: true,
			expectKeyFile:  false,
		}),
		Entry("SFTP with SSH key only", sftpPasswordConfigTestCase{
			secretData: map[string][]byte{
				"SFTP_HOST":     []byte("sftp.example.com"),
				"SFTP_USERNAME": []byte("user"),
				"SFTP_KEY_FILE": []byte("ssh-private-key-content"),
				"SFTP_PATH":     []byte("/backup"),
			},
			expectPassword: false,
			expectKeyFile:  true,
		}),
		Entry("SFTP with both password and SSH key (key takes precedence)", sftpPasswordConfigTestCase{
			secretData: map[string][]byte{
				"SFTP_HOST":     []byte("sftp.example.com"),
				"SFTP_USERNAME": []byte("user"),
				"SFTP_PASSWORD": []byte("secret-pass"),
				"SFTP_KEY_FILE": []byte("ssh-private-key-content"),
				"SFTP_PATH":     []byte("/backup"),
			},
			expectPassword: true,
			expectKeyFile:  true,
		}),
	)
})

var _ = Describe("SFTP Known Hosts Configuration", func() {
	type sftpKnownHostsTestCase struct {
		secretData           map[string][]byte
		expectKnownHosts     bool
		expectKnownHostsData bool
	}

	DescribeTable("properly handles known hosts environment variables",
		func(tc sftpKnownHostsTestCase) {
			owner := &volsyncv1alpha1.ReplicationSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-source",
					Namespace: "test-namespace",
				},
			}

			mover := &Mover{
				logger:   logr.Discard(),
				owner:    owner,
				username: "test-user",
				hostname: "test-host",
			}

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-secret",
				},
				Data: tc.secretData,
			}

			envVars := mover.buildEnvironmentVariables(secret)

			// Check SFTP_KNOWN_HOSTS
			// Note: Both env vars are always present, but only have values if they exist in the secret
			foundKnownHosts := false
			foundKnownHostsData := false
			for _, env := range envVars {
				if env.Name == "SFTP_KNOWN_HOSTS" {
					foundKnownHosts = true
					// Verify it's optional
					Expect(env.ValueFrom).NotTo(BeNil(), "SFTP_KNOWN_HOSTS should have ValueFrom")
					Expect(env.ValueFrom.SecretKeyRef).NotTo(BeNil(), "SFTP_KNOWN_HOSTS should have SecretKeyRef")
					Expect(*env.ValueFrom.SecretKeyRef.Optional).To(BeTrue(), "SFTP_KNOWN_HOSTS should be optional")
				}
				if env.Name == "SFTP_KNOWN_HOSTS_DATA" {
					foundKnownHostsData = true
					// Verify it's optional
					Expect(env.ValueFrom).NotTo(BeNil(), "SFTP_KNOWN_HOSTS_DATA should have ValueFrom")
					Expect(env.ValueFrom.SecretKeyRef).NotTo(BeNil(), "SFTP_KNOWN_HOSTS_DATA should have SecretKeyRef")
					Expect(*env.ValueFrom.SecretKeyRef.Optional).To(BeTrue(), "SFTP_KNOWN_HOSTS_DATA should be optional")
				}
			}

			// Both variables should always be present
			Expect(foundKnownHosts).To(BeTrue(), "SFTP_KNOWN_HOSTS environment variable should always be present")
			Expect(foundKnownHostsData).To(BeTrue(), "SFTP_KNOWN_HOSTS_DATA environment variable should always be present")
		},
		Entry("SFTP with known hosts file", sftpKnownHostsTestCase{
			secretData: map[string][]byte{
				"SFTP_HOST":        []byte("sftp.example.com"),
				"SFTP_USERNAME":    []byte("user"),
				"SFTP_PASSWORD":    []byte("secret-pass"),
				"SFTP_KNOWN_HOSTS": []byte("/etc/ssh/known_hosts"),
			},
			expectKnownHosts:     true,
			expectKnownHostsData: false,
		}),
		Entry("SFTP with known hosts data", sftpKnownHostsTestCase{
			secretData: map[string][]byte{
				"SFTP_HOST":             []byte("sftp.example.com"),
				"SFTP_USERNAME":         []byte("user"),
				"SFTP_PASSWORD":         []byte("secret-pass"),
				"SFTP_KNOWN_HOSTS_DATA": []byte("sftp.example.com ssh-rsa AAAAB3NzaC1yc2E..."),
			},
			expectKnownHosts:     false,
			expectKnownHostsData: true,
		}),
		Entry("SFTP with both known hosts file and data", sftpKnownHostsTestCase{
			secretData: map[string][]byte{
				"SFTP_HOST":             []byte("sftp.example.com"),
				"SFTP_USERNAME":         []byte("user"),
				"SFTP_PASSWORD":         []byte("secret-pass"),
				"SFTP_KNOWN_HOSTS":      []byte("/etc/ssh/known_hosts"),
				"SFTP_KNOWN_HOSTS_DATA": []byte("sftp.example.com ssh-rsa AAAAB3NzaC1yc2E..."),
			},
			expectKnownHosts:     true,
			expectKnownHostsData: true,
		}),
		Entry("SFTP without known hosts", sftpKnownHostsTestCase{
			secretData: map[string][]byte{
				"SFTP_HOST":     []byte("sftp.example.com"),
				"SFTP_USERNAME": []byte("user"),
				"SFTP_PASSWORD": []byte("secret-pass"),
			},
			expectKnownHosts:     false,
			expectKnownHostsData: false,
		}),
	)
})

var _ = Describe("Additional Args With All Repository Types", func() {
	type repoTypeTestCase struct {
		name       string
		secretData map[string][]byte
	}

	additionalArgs := []string{
		"--one-file-system",
		"--parallel=8",
		"--compression=zstd",
	}

	DescribeTable("properly includes KOPIA_ADDITIONAL_ARGS for all repository types",
		func(tc repoTypeTestCase) {
			owner := &volsyncv1alpha1.ReplicationSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-source",
					Namespace: "test-namespace",
				},
			}

			mover := &Mover{
				logger:         logr.Discard(),
				owner:          owner,
				username:       "test-user",
				hostname:       "test-host",
				additionalArgs: additionalArgs,
			}

			// Add common password
			tc.secretData["KOPIA_PASSWORD"] = []byte("test-password")

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-secret",
				},
				Data: tc.secretData,
			}

			envVars := mover.buildEnvironmentVariables(secret)

			// Check that KOPIA_ADDITIONAL_ARGS is included
			foundAdditionalArgs := false
			for _, env := range envVars {
				if env.Name == "KOPIA_ADDITIONAL_ARGS" {
					foundAdditionalArgs = true
					expected := "--one-file-system|VOLSYNC_ARG_SEP|--parallel=8|VOLSYNC_ARG_SEP|--compression=zstd"
					Expect(env.Value).To(Equal(expected),
						"Expected KOPIA_ADDITIONAL_ARGS value '%s', got '%s'", expected, env.Value)
					break
				}
			}

			Expect(foundAdditionalArgs).To(BeTrue(),
				"KOPIA_ADDITIONAL_ARGS not found for %s", tc.name)
		},
		Entry("S3 repository", repoTypeTestCase{
			name: "S3 repository",
			secretData: map[string][]byte{
				"KOPIA_S3_BUCKET":       []byte("my-bucket"),
				"AWS_ACCESS_KEY_ID":     []byte("access-key"),
				"AWS_SECRET_ACCESS_KEY": []byte("secret-key"),
			},
		}),
		Entry("Azure repository", repoTypeTestCase{
			name: "Azure repository",
			secretData: map[string][]byte{
				"KOPIA_AZURE_CONTAINER":       []byte("my-container"),
				"KOPIA_AZURE_STORAGE_ACCOUNT": []byte("storage-account"),
				"KOPIA_AZURE_STORAGE_KEY":     []byte("storage-key"),
			},
		}),
		Entry("GCS repository", repoTypeTestCase{
			name: "GCS repository",
			secretData: map[string][]byte{
				"KOPIA_GCS_BUCKET":               []byte("my-bucket"),
				"GOOGLE_APPLICATION_CREDENTIALS": []byte(`{"type": "service_account"}`),
			},
		}),
		Entry("Filesystem repository", repoTypeTestCase{
			name: "Filesystem repository",
			secretData: map[string][]byte{
				"KOPIA_REPOSITORY": []byte("filesystem:///backup"),
			},
		}),
		Entry("B2 repository", repoTypeTestCase{
			name: "B2 repository",
			secretData: map[string][]byte{
				"KOPIA_B2_BUCKET":    []byte("my-bucket"),
				"B2_ACCOUNT_ID":      []byte("account-id"),
				"B2_APPLICATION_KEY": []byte("app-key"),
			},
		}),
		Entry("WebDAV repository", repoTypeTestCase{
			name: "WebDAV repository",
			secretData: map[string][]byte{
				"WEBDAV_URL":      []byte("https://webdav.example.com"),
				"WEBDAV_USERNAME": []byte("user"),
				"WEBDAV_PASSWORD": []byte("pass"),
			},
		}),
		Entry("SFTP repository", repoTypeTestCase{
			name: "SFTP repository",
			secretData: map[string][]byte{
				"SFTP_HOST":     []byte("sftp.example.com"),
				"SFTP_USERNAME": []byte("user"),
				"SFTP_PASSWORD": []byte("pass"),
				"SFTP_PATH":     []byte("/backup"),
			},
		}),
		Entry("Rclone repository", repoTypeTestCase{
			name: "Rclone repository",
			secretData: map[string][]byte{
				"RCLONE_REMOTE_PATH": []byte("remote:path"),
				"RCLONE_CONFIG":      []byte("[remote]\ntype = s3\n..."),
			},
		}),
		Entry("Google Drive repository", repoTypeTestCase{
			name: "Google Drive repository",
			secretData: map[string][]byte{
				"GOOGLE_DRIVE_FOLDER_ID":   []byte("folder-id"),
				"GOOGLE_DRIVE_CREDENTIALS": []byte(`{"type": "oauth2"}`),
			},
		}),
	)
})

var _ = Describe("Execute Repository Command Function", func() {
	// This test validates that the execute_repository_command function in entry.sh
	// would properly apply KOPIA_ADDITIONAL_ARGS for all repository types
	// The actual shell script testing would require a different approach (e.g., BATS)
	// but we can test that the Go code sets up the environment correctly

	type executeRepoCommandTestCase struct {
		additionalArgs []string
		connectionType string // "direct", "json", "legacy"
		expectEnvVar   bool
	}

	DescribeTable("sets up environment correctly for execute_repository_command",
		func(tc executeRepoCommandTestCase) {
			owner := &volsyncv1alpha1.ReplicationSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-source",
					Namespace: "test-namespace",
				},
			}

			mover := &Mover{
				logger:         logr.Discard(),
				owner:          owner,
				username:       "test-user",
				hostname:       "test-host",
				additionalArgs: tc.additionalArgs,
			}

			secretData := map[string][]byte{
				"KOPIA_PASSWORD": []byte("test-password"),
			}

			// Add connection-specific data
			switch tc.connectionType {
			case "json":
				secretData["KOPIA_CONFIG_JSON"] = []byte(`{"repository": {"s3": {"bucket": "test"}}}`)
			case "legacy":
				secretData["KOPIA_CONFIG_PATH"] = []byte("/config/kopia")
			default: // direct
				secretData["KOPIA_S3_BUCKET"] = []byte("test-bucket")
			}

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-secret",
				},
				Data: secretData,
			}

			envVars := mover.buildEnvironmentVariables(secret)

			foundAdditionalArgs := false
			for _, env := range envVars {
				if env.Name == "KOPIA_ADDITIONAL_ARGS" {
					foundAdditionalArgs = true
					break
				}
			}

			Expect(foundAdditionalArgs).To(Equal(tc.expectEnvVar),
				"KOPIA_ADDITIONAL_ARGS presence: expected %v, got %v", tc.expectEnvVar, foundAdditionalArgs)
		},
		Entry("Direct connection with additional args", executeRepoCommandTestCase{
			additionalArgs: []string{"--one-file-system"},
			connectionType: "direct",
			expectEnvVar:   true,
		}),
		Entry("JSON config with additional args", executeRepoCommandTestCase{
			additionalArgs: []string{"--parallel=8"},
			connectionType: "json",
			expectEnvVar:   true,
		}),
		Entry("Legacy config with additional args", executeRepoCommandTestCase{
			additionalArgs: []string{"--compression=zstd"},
			connectionType: "legacy",
			expectEnvVar:   true,
		}),
		Entry("No additional args", executeRepoCommandTestCase{
			additionalArgs: nil,
			connectionType: "direct",
			expectEnvVar:   false,
		}),
	)
})

var _ = Describe("SFTP Port Configuration", func() {
	type sftpPortTestCase struct {
		secretData map[string][]byte
		expectPort bool
	}

	DescribeTable("properly handles SFTP_PORT",
		func(tc sftpPortTestCase) {
			owner := &volsyncv1alpha1.ReplicationSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-source",
					Namespace: "test-namespace",
				},
			}

			mover := &Mover{
				logger:   logr.Discard(),
				owner:    owner,
				username: "test-user",
				hostname: "test-host",
			}

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-secret",
				},
				Data: tc.secretData,
			}

			envVars := mover.buildEnvironmentVariables(secret)

			foundPort := false
			for _, env := range envVars {
				if env.Name == "SFTP_PORT" {
					foundPort = true
					if tc.expectPort {
						// Verify it references the secret
						Expect(env.ValueFrom).NotTo(BeNil(), "SFTP_PORT should be from secret")
						Expect(env.ValueFrom.SecretKeyRef).NotTo(BeNil(), "SFTP_PORT should be from secret")
					}
					break
				}
			}

			// SFTP_PORT env var is always present, but only has a value if it exists in the secret
			Expect(foundPort).To(BeTrue(), "SFTP_PORT environment variable should always be present")
		},
		Entry("SFTP with custom port", sftpPortTestCase{
			secretData: map[string][]byte{
				"SFTP_HOST":     []byte("sftp.example.com"),
				"SFTP_USERNAME": []byte("user"),
				"SFTP_PASSWORD": []byte("pass"),
				"SFTP_PORT":     []byte("2222"),
			},
			expectPort: true,
		}),
		Entry("SFTP without custom port (uses default 22)", sftpPortTestCase{
			secretData: map[string][]byte{
				"SFTP_HOST":     []byte("sftp.example.com"),
				"SFTP_USERNAME": []byte("user"),
				"SFTP_PASSWORD": []byte("pass"),
			},
			expectPort: false,
		}),
	)
})

var _ = Describe("Credential Precedence", func() {
	It("makes both SSH key and password available when both are provided", func() {
		// Test that when both SSH key and password are provided,
		// both are made available (entry.sh will handle precedence)
		secretData := map[string][]byte{
			"SFTP_HOST":     []byte("sftp.example.com"),
			"SFTP_USERNAME": []byte("user"),
			"SFTP_PASSWORD": []byte("password"),
			"SFTP_KEY_FILE": []byte("ssh-key-content"),
			"SFTP_PATH":     []byte("/backup"),
		}

		owner := &volsyncv1alpha1.ReplicationSource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-source",
				Namespace: "test-namespace",
			},
		}

		mover := &Mover{
			logger:   logr.Discard(),
			owner:    owner,
			username: "test-user",
			hostname: "test-host",
		}

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-secret",
			},
			Data: secretData,
		}

		// Build pod spec
		podSpec := &corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:  "kopia",
				Image: "test-image",
				Env:   []corev1.EnvVar{},
			}},
			Volumes: []corev1.Volume{},
		}

		// Configure credentials
		mover.configureCredentials(podSpec, secret)

		// Build environment variables
		envVars := mover.buildEnvironmentVariables(secret)
		podSpec.Containers[0].Env = envVars

		// Check both password and key are available
		foundPassword := false
		foundKeyEnv := false
		for _, env := range envVars {
			if env.Name == "SFTP_PASSWORD" {
				foundPassword = true
			}
			if env.Name == "SFTP_KEY_FILE" {
				foundKeyEnv = true
			}
		}

		Expect(foundPassword).To(BeTrue(), "SFTP_PASSWORD should be available even when SSH key is present")
		Expect(foundKeyEnv).To(BeTrue(), "SFTP_KEY_FILE environment variable should be set")

		// Check that SSH key is mounted
		foundKeyMount := false
		for _, mount := range podSpec.Containers[0].VolumeMounts {
			if mount.MountPath == "/credentials" {
				foundKeyMount = true
				break
			}
		}

		Expect(foundKeyMount).To(BeTrue(), "SSH key should be mounted at /credentials")
	})
})

var _ = Describe("Backend Environment Variables Complete", func() {
	It("includes all required SFTP backend variables", func() {
		owner := &volsyncv1alpha1.ReplicationSource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-source",
				Namespace: "test-namespace",
			},
		}

		mover := &Mover{
			logger:   logr.Discard(),
			owner:    owner,
			username: "test-user",
			hostname: "test-host",
		}

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-secret",
			},
			Data: map[string][]byte{
				"KOPIA_PASSWORD": []byte("test-password"),
			},
		}

		envVars := mover.buildEnvironmentVariables(secret)

		// List of all expected SFTP-related variables
		expectedSFTPVars := []string{
			"SFTP_HOST",
			"SFTP_USERNAME",
			"SFTP_KEY_FILE",
			"SFTP_PORT",
			"SFTP_PASSWORD",
			"SFTP_PATH",
			"SFTP_KNOWN_HOSTS",
			"SFTP_KNOWN_HOSTS_DATA",
		}

		// Check that all SFTP variables are present in the environment
		for _, varName := range expectedSFTPVars {
			found := false
			for _, env := range envVars {
				if env.Name == varName {
					found = true
					// Verify they reference the secret appropriately
					Expect(env.ValueFrom).NotTo(BeNil(), "%s should reference secret", varName)
					Expect(env.ValueFrom.SecretKeyRef).NotTo(BeNil(), "%s should reference secret", varName)
					Expect(env.ValueFrom.SecretKeyRef.Name).To(Equal(secret.Name),
						"%s should reference secret %s", varName, secret.Name)
					Expect(env.ValueFrom.SecretKeyRef.Key).To(Equal(varName),
						"%s should reference correct key", varName)
					// All SFTP variables should be optional
					Expect(*env.ValueFrom.SecretKeyRef.Optional).To(BeTrue(),
						"%s should be optional", varName)
					break
				}
			}
			Expect(found).To(BeTrue(), "Expected SFTP environment variable %s not found", varName)
		}
	})
})

var _ = Describe("Manual Config With Additional Args", func() {
	type manualConfigTestCase struct {
		secretData     map[string][]byte
		additionalArgs []string
	}

	DescribeTable("properly handles additional args with manual configuration",
		func(tc manualConfigTestCase) {
			owner := &volsyncv1alpha1.ReplicationSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-source",
					Namespace: "test-namespace",
				},
			}

			mover := &Mover{
				logger:         logr.Discard(),
				owner:          owner,
				username:       "test-user",
				hostname:       "test-host",
				additionalArgs: tc.additionalArgs,
			}

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-secret",
				},
				Data: tc.secretData,
			}

			envVars := mover.buildEnvironmentVariables(secret)

			// Verify KOPIA_ADDITIONAL_ARGS is set
			foundAdditionalArgs := false
			for _, env := range envVars {
				if env.Name == "KOPIA_ADDITIONAL_ARGS" {
					foundAdditionalArgs = true
					if len(tc.additionalArgs) > 0 {
						Expect(env.Value).NotTo(BeEmpty(), "KOPIA_ADDITIONAL_ARGS should not be empty")
					}
					break
				}
			}

			if len(tc.additionalArgs) > 0 {
				Expect(foundAdditionalArgs).To(BeTrue(), "Expected KOPIA_ADDITIONAL_ARGS for manual config")
			}
		},
		Entry("Manual config with additional args", manualConfigTestCase{
			secretData: map[string][]byte{
				"KOPIA_CONFIG_PATH": []byte("/config/kopia"),
				"KOPIA_PASSWORD":    []byte("test-password"),
			},
			additionalArgs: []string{"--one-file-system", "--parallel=8"},
		}),
		Entry("JSON config with additional args", manualConfigTestCase{
			secretData: map[string][]byte{
				"KOPIA_CONFIG_JSON": []byte(`{"repository": {"s3": {"bucket": "test"}}}`),
				"KOPIA_PASSWORD":    []byte("test-password"),
			},
			additionalArgs: []string{"--compression=zstd"},
		}),
	)
})
