//go:build !disable_kopia && unit

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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

var _ = Describe("Credential Configuration", func() {
	Describe("configureCredentials", func() {
		type credentialTestCase struct {
			secretData  map[string][]byte
			expectEnvs  []string
			expectFiles []string
			expectMount bool
		}

		DescribeTable("configures credentials correctly",
			func(tc credentialTestCase) {
				// Setup
				mover := &Mover{}
				podSpec := &corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "kopia",
						Image: "test-image",
						Env:   []corev1.EnvVar{},
					}},
					Volumes: []corev1.Volume{},
				}
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-secret",
					},
					Data: tc.secretData,
				}

				// Execute
				mover.configureCredentials(podSpec, secret)

				// Verify environment variables
				for _, expectEnv := range tc.expectEnvs {
					found := false
					for _, env := range podSpec.Containers[0].Env {
						if env.Name == expectEnv {
							found = true
							break
						}
					}
					Expect(found).To(BeTrue(), "Expected environment variable %s not found", expectEnv)
				}

				// Verify volume mount presence
				credentialMountCount := 0
				for _, mount := range podSpec.Containers[0].VolumeMounts {
					if mount.MountPath == "/credentials" {
						credentialMountCount++
					}
				}

				if tc.expectMount {
					Expect(credentialMountCount).To(Equal(1), "Expected exactly 1 credential mount")

					// Verify volume configuration
					Expect(podSpec.Volumes).To(HaveLen(1), "Expected exactly 1 volume")

					volume := podSpec.Volumes[0]
					Expect(volume.Name).To(Equal("credentials"), "Expected volume name 'credentials'")
					Expect(volume.VolumeSource.Secret).NotTo(BeNil(), "Expected secret volume source")
					Expect(volume.VolumeSource.Secret.Items).To(HaveLen(len(tc.expectFiles)),
						"Expected %d credential files", len(tc.expectFiles))

					// Verify file permissions
					for _, item := range volume.VolumeSource.Secret.Items {
						if item.Path == "sftp_key" {
							Expect(item.Mode).NotTo(BeNil(), "Expected SSH key to have mode set")
							Expect(*item.Mode).To(Equal(int32(0600)), "Expected SSH key to have mode 0600")
						}
					}

					// Verify default mode for security
					Expect(volume.VolumeSource.Secret.DefaultMode).NotTo(BeNil(), "Expected default mode to be set")
					Expect(*volume.VolumeSource.Secret.DefaultMode).To(Equal(int32(0400)), "Expected default mode 0400")
				} else {
					Expect(credentialMountCount).To(Equal(0), "Expected no credential mounts")
					Expect(podSpec.Volumes).To(BeEmpty(), "Expected no volumes")
				}
			},
			Entry("GCS credentials only", credentialTestCase{
				name: "GCS credentials only",
				secretData: map[string][]byte{
					"GOOGLE_APPLICATION_CREDENTIALS": []byte(`{"type": "service_account"}`),
				},
				expectEnvs:  []string{"GOOGLE_APPLICATION_CREDENTIALS"},
				expectFiles: []string{"gcs.json"},
				expectMount: true,
			}),
			Entry("SFTP credentials only", credentialTestCase{
				name: "SFTP credentials only",
				secretData: map[string][]byte{
					"SFTP_KEY_FILE": []byte("ssh-private-key-content"),
				},
				expectEnvs:  []string{"SFTP_KEY_FILE"},
				expectFiles: []string{"sftp_key"},
				expectMount: true,
			}),
			Entry("Multiple credentials", credentialTestCase{
				name: "Multiple credentials",
				secretData: map[string][]byte{
					"GOOGLE_APPLICATION_CREDENTIALS": []byte(`{"type": "service_account"}`),
					"GOOGLE_DRIVE_CREDENTIALS":       []byte(`{"type": "oauth2"}`),
					"SFTP_KEY_FILE":                  []byte("ssh-private-key-content"),
				},
				expectEnvs:  []string{"GOOGLE_APPLICATION_CREDENTIALS", "GOOGLE_DRIVE_CREDENTIALS", "SFTP_KEY_FILE"},
				expectFiles: []string{"gcs.json", "gdrive.json", "sftp_key"},
				expectMount: true,
			}),
			Entry("No credentials", credentialTestCase{
				name:        "No credentials",
				secretData:  map[string][]byte{},
				expectEnvs:  []string{},
				expectFiles: []string{},
				expectMount: false,
			}),
		)
	})

	Describe("buildEnvironmentVariables", func() {
		It("should include username and hostname overrides", func() {
			// Create a mock owner for the mover - use ReplicationSource which is the correct type
			owner := &volsyncv1alpha1.ReplicationSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-owner",
					Namespace: "test-namespace",
				},
			}

			mover := &Mover{
				username: "test-user",
				hostname: "test-host",
				owner:    owner,
			}
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-secret",
				},
				Data: map[string][]byte{
					"KOPIA_REPOSITORY": []byte("s3://test-bucket/path"),
					"KOPIA_PASSWORD":   []byte("test-password"),
				},
			}

			envVars := mover.buildEnvironmentVariables(secret)

			// Check username and hostname overrides
			envMap := make(map[string]string)
			for _, env := range envVars {
				envMap[env.Name] = env.Value
			}

			Expect(envMap).To(HaveKeyWithValue("KOPIA_OVERRIDE_USERNAME", "test-user"))
			Expect(envMap).To(HaveKeyWithValue("KOPIA_OVERRIDE_HOSTNAME", "test-host"))
		})

		It("should include all required backend environment variables", func() {
			owner := &volsyncv1alpha1.ReplicationSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-owner",
					Namespace: "test-namespace",
				},
			}

			mover := &Mover{
				username: "test-user",
				hostname: "test-host",
				owner:    owner,
			}
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-secret",
				},
				Data: map[string][]byte{
					"KOPIA_REPOSITORY": []byte("s3://test-bucket/path"),
					"KOPIA_PASSWORD":   []byte("test-password"),
				},
			}

			envVars := mover.buildEnvironmentVariables(secret)

			// Build a set of env var names
			envNames := make(map[string]bool)
			for _, env := range envVars {
				envNames[env.Name] = true
			}

			// Check that all backend environment variables are included
			requiredBackendVars := []string{
				"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "KOPIA_S3_BUCKET",
				"KOPIA_S3_ENDPOINT", "AWS_S3_ENDPOINT", "KOPIA_S3_DISABLE_TLS", "AWS_S3_DISABLE_TLS",
				"AWS_REGION", "AWS_DEFAULT_REGION", "AWS_SESSION_TOKEN", "AWS_PROFILE",
				"AZURE_ACCOUNT_NAME", "AZURE_ACCOUNT_KEY", "KOPIA_AZURE_CONTAINER",
				"KOPIA_AZURE_STORAGE_ACCOUNT", "AZURE_STORAGE_ACCOUNT",
				"KOPIA_AZURE_STORAGE_KEY", "AZURE_STORAGE_KEY", "AZURE_STORAGE_SAS_TOKEN",
				"AZURE_ACCOUNT_SAS", "AZURE_ENDPOINT_SUFFIX",
				"KOPIA_GCS_BUCKET", "GOOGLE_APPLICATION_CREDENTIALS", "GOOGLE_PROJECT_ID",
				"KOPIA_B2_BUCKET", "B2_ACCOUNT_ID", "B2_APPLICATION_KEY",
				"WEBDAV_URL", "WEBDAV_USERNAME", "WEBDAV_PASSWORD",
				"SFTP_HOST", "SFTP_USERNAME", "SFTP_KEY_FILE", "SFTP_PORT", "SFTP_PASSWORD", "SFTP_PATH",
				"RCLONE_REMOTE_PATH", "RCLONE_EXE", "RCLONE_CONFIG",
				"GOOGLE_DRIVE_FOLDER_ID", "GOOGLE_DRIVE_CREDENTIALS",
			}

			for _, requiredVar := range requiredBackendVars {
				Expect(envNames).To(HaveKey(requiredVar),
					"Required backend environment variable %s not found", requiredVar)
			}
		})
	})

	Describe("SSH key permissions", func() {
		It("should set correct permissions for SSH key files", func() {
			mover := &Mover{}
			podSpec := &corev1.PodSpec{
				Containers: []corev1.Container{{
					Name:  "kopia",
					Image: "test-image",
					Env:   []corev1.EnvVar{},
				}},
				Volumes: []corev1.Volume{},
			}
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-secret",
				},
				Data: map[string][]byte{
					"SFTP_KEY_FILE": []byte("ssh-private-key-content"),
				},
			}

			mover.configureCredentials(podSpec, secret)

			// Find the SSH key file in the volume items
			Expect(podSpec.Volumes).NotTo(BeEmpty())
			volume := podSpec.Volumes[0]

			var sshKeyFound bool
			for _, item := range volume.VolumeSource.Secret.Items {
				if item.Path == "sftp_key" {
					sshKeyFound = true
					Expect(item.Mode).NotTo(BeNil(), "Expected SSH key file mode to be set")
					Expect(*item.Mode).To(Equal(int32(0600)), "Expected SSH key file mode to be 0600")
					break
				}
			}
			Expect(sshKeyFound).To(BeTrue(), "SSH key file not found in volume items")
		})
	})
})
