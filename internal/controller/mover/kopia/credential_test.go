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
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

// TestCredentialConfigurationUnit performs unit tests for credential configuration
// without requiring the full Kubernetes test environment
func TestCredentialConfigurationUnit(t *testing.T) {
	tests := []struct {
		name        string
		secretData  map[string][]byte
		expectEnvs  []string
		expectFiles []string
		expectMount bool
	}{
		{
			name: "GCS credentials only",
			secretData: map[string][]byte{
				"GOOGLE_APPLICATION_CREDENTIALS": []byte(`{"type": "service_account"}`),
			},
			expectEnvs:  []string{"GOOGLE_APPLICATION_CREDENTIALS"},
			expectFiles: []string{"gcs.json"},
			expectMount: true,
		},
		{
			name: "SFTP credentials only",
			secretData: map[string][]byte{
				"SFTP_KEY_FILE": []byte("ssh-private-key-content"),
			},
			expectEnvs:  []string{"SFTP_KEY_FILE"},
			expectFiles: []string{"sftp_key"},
			expectMount: true,
		},
		{
			name: "Multiple credentials",
			secretData: map[string][]byte{
				"GOOGLE_APPLICATION_CREDENTIALS": []byte(`{"type": "service_account"}`),
				"GOOGLE_DRIVE_CREDENTIALS":       []byte(`{"type": "oauth2"}`),
				"SFTP_KEY_FILE":                  []byte("ssh-private-key-content"),
			},
			expectEnvs:  []string{"GOOGLE_APPLICATION_CREDENTIALS", "GOOGLE_DRIVE_CREDENTIALS", "SFTP_KEY_FILE"},
			expectFiles: []string{"gcs.json", "gdrive.json", "sftp_key"},
			expectMount: true,
		},
		{
			name:        "No credentials",
			secretData:  map[string][]byte{},
			expectEnvs:  []string{},
			expectFiles: []string{},
			expectMount: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
				Data: tt.secretData,
			}

			// Execute
			mover.configureCredentials(podSpec, secret)

			// Verify environment variables
			for _, expectEnv := range tt.expectEnvs {
				found := false
				for _, env := range podSpec.Containers[0].Env {
					if env.Name == expectEnv {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected environment variable %s not found", expectEnv)
				}
			}

			// Verify volume mount presence
			credentialMountCount := 0
			for _, mount := range podSpec.Containers[0].VolumeMounts {
				if mount.MountPath == "/credentials" {
					credentialMountCount++
				}
			}

			if tt.expectMount {
				if credentialMountCount != 1 {
					t.Errorf("Expected exactly 1 credential mount, got %d", credentialMountCount)
				}
			} else {
				if credentialMountCount != 0 {
					t.Errorf("Expected no credential mounts, got %d", credentialMountCount)
				}
			}

			// Verify volume configuration
			if tt.expectMount {
				if len(podSpec.Volumes) != 1 {
					t.Errorf("Expected exactly 1 volume, got %d", len(podSpec.Volumes))
				} else {
					volume := podSpec.Volumes[0]
					if volume.Name != "credentials" {
						t.Errorf("Expected volume name 'credentials', got %s", volume.Name)
					}
					if volume.VolumeSource.Secret == nil {
						t.Errorf("Expected secret volume source")
					} else {
						if len(volume.VolumeSource.Secret.Items) != len(tt.expectFiles) {
							t.Errorf("Expected %d credential files, got %d", len(tt.expectFiles), len(volume.VolumeSource.Secret.Items))
						}

						// Verify file permissions
						for _, item := range volume.VolumeSource.Secret.Items {
							if item.Path == "sftp_key" {
								if item.Mode == nil || *item.Mode != 0600 {
									t.Errorf("Expected SSH key to have mode 0600, got %v", item.Mode)
								}
							}
						}

						// Verify default mode for security
						if volume.VolumeSource.Secret.DefaultMode == nil || *volume.VolumeSource.Secret.DefaultMode != 0400 {
							t.Errorf("Expected default mode 0400, got %v", volume.VolumeSource.Secret.DefaultMode)
						}
					}
				}
			} else {
				if len(podSpec.Volumes) != 0 {
					t.Errorf("Expected no volumes, got %d", len(podSpec.Volumes))
				}
			}
		})
	}
}

// TestEnvironmentVariables tests that all required environment variables are included
func TestEnvironmentVariables(t *testing.T) {
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
	foundUsername := false
	foundHostname := false
	for _, env := range envVars {
		if env.Name == "KOPIA_OVERRIDE_USERNAME" && env.Value == "test-user" {
			foundUsername = true
		}
		if env.Name == "KOPIA_OVERRIDE_HOSTNAME" && env.Value == "test-host" {
			foundHostname = true
		}
	}

	if !foundUsername {
		t.Error("Expected KOPIA_OVERRIDE_USERNAME environment variable")
	}
	if !foundHostname {
		t.Error("Expected KOPIA_OVERRIDE_HOSTNAME environment variable")
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
		"KOPIA_FS_PATH",
	}

	for _, requiredVar := range requiredBackendVars {
		found := false
		for _, env := range envVars {
			if env.Name == requiredVar {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Required backend environment variable %s not found", requiredVar)
		}
	}
}

// Helper function to check if a pointer to int32 has the expected value
func int32PtrEqual(a *int32, b int32) bool {
	if a == nil {
		return false
	}
	return *a == b
}

// Test that SSH key file permissions are set correctly
func TestSSHKeyPermissions(t *testing.T) {
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
	volume := podSpec.Volumes[0]
	for _, item := range volume.VolumeSource.Secret.Items {
		if item.Path == "sftp_key" {
			if !int32PtrEqual(item.Mode, 0600) {
				t.Errorf("Expected SSH key file mode to be 0600, got %v", item.Mode)
			}
			return
		}
	}
	t.Error("SSH key file not found in volume items")
}
