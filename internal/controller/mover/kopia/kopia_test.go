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
	"flag"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

var _ = Describe("Kopia", func() {
	var ns *corev1.Namespace
	logger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))

	BeforeEach(func() {
		// Create namespace for test
		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "vh-",
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		Expect(ns.Name).NotTo(BeEmpty())
	})

	AfterEach(func() {
		// All resources are namespaced, so this should clean it all up
		Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
	})

	Context("Kopia mover builder", func() {
		It("should have correct name", func() {
			viper := viper.New()
			flags := flag.NewFlagSet("test", flag.ContinueOnError)
			b, err := newBuilder(viper, flags)
			Expect(err).NotTo(HaveOccurred())
			Expect(b.Name()).To(Equal("kopia"))
		})

		It("should return version info", func() {
			viper := viper.New()
			flags := flag.NewFlagSet("test", flag.ContinueOnError)
			b, err := newBuilder(viper, flags)
			Expect(err).NotTo(HaveOccurred())
			versionInfo := b.VersionInfo()
			Expect(versionInfo).To(ContainSubstring("Kopia container"))
		})

		It("should return nil mover when spec is nil", func() {
			viper := viper.New()
			flags := flag.NewFlagSet("test", flag.ContinueOnError)
			b, err := newBuilder(viper, flags)
			Expect(err).NotTo(HaveOccurred())

			rs := &volsyncv1alpha1.ReplicationSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rs",
					Namespace: ns.Name,
				},
				Spec: volsyncv1alpha1.ReplicationSourceSpec{
					SourcePVC: "test",
				},
			}

			m, err := b.FromSource(k8sClient, logger, &events.FakeRecorder{}, rs, false)
			Expect(err).To(BeNil())
			Expect(m).To(BeNil())

			rd := &volsyncv1alpha1.ReplicationDestination{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rd",
					Namespace: ns.Name,
				},
			}

			m, err = b.FromDestination(k8sClient, logger, &events.FakeRecorder{}, rd, false)
			Expect(err).To(BeNil())
			Expect(m).To(BeNil())
		})

		It("should create mover from source", func() {
			viper := viper.New()
			flags := flag.NewFlagSet("test", flag.ContinueOnError)
			b, err := newBuilder(viper, flags)
			Expect(err).NotTo(HaveOccurred())

			rs := &volsyncv1alpha1.ReplicationSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rs",
					Namespace: ns.Name,
				},
				Spec: volsyncv1alpha1.ReplicationSourceSpec{
					SourcePVC: "test",
					Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
						Repository: "kopia-secret",
						ReplicationSourceVolumeOptions: volsyncv1alpha1.ReplicationSourceVolumeOptions{
							CopyMethod: volsyncv1alpha1.CopyMethodSnapshot,
						},
					},
				},
				Status: &volsyncv1alpha1.ReplicationSourceStatus{},
			}

			m, err := b.FromSource(k8sClient, logger, &events.FakeRecorder{}, rs, false)
			Expect(err).To(BeNil())
			Expect(m).NotTo(BeNil())
			Expect(m.Name()).To(Equal("kopia"))
		})

		It("should create mover from destination", func() {
			viper := viper.New()
			flags := flag.NewFlagSet("test", flag.ContinueOnError)
			b, err := newBuilder(viper, flags)
			Expect(err).NotTo(HaveOccurred())

			rd := &volsyncv1alpha1.ReplicationDestination{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rd",
					Namespace: ns.Name,
				},
				Spec: volsyncv1alpha1.ReplicationDestinationSpec{
					Kopia: &volsyncv1alpha1.ReplicationDestinationKopiaSpec{
						Repository: "kopia-secret",
						ReplicationDestinationVolumeOptions: volsyncv1alpha1.ReplicationDestinationVolumeOptions{
							Capacity: ptr.To(resource.MustParse("1Gi")),
						},
					},
				},
				Status: &volsyncv1alpha1.ReplicationDestinationStatus{},
			}

			m, err := b.FromDestination(k8sClient, logger, &events.FakeRecorder{}, rd, false)
			Expect(err).To(BeNil())
			Expect(m).NotTo(BeNil())
			Expect(m.Name()).To(Equal("kopia"))
		})
	})

	Context("Kopia credential configuration", func() {
		var mover *Mover
		var podSpec *corev1.PodSpec
		var secret *corev1.Secret

		BeforeEach(func() {
			// Setup basic mover and pod spec for testing
			mover = &Mover{}
			podSpec = &corev1.PodSpec{
				Containers: []corev1.Container{{
					Name:  "kopia",
					Image: "test-image",
					Env:   []corev1.EnvVar{},
				}},
				Volumes: []corev1.Volume{},
			}
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: ns.Name,
				},
				Data: map[string][]byte{},
			}
		})

		It("should configure GCS credentials only", func() {
			secret.Data["GOOGLE_APPLICATION_CREDENTIALS"] = []byte(`{"type": "service_account"}`)

			mover.configureCredentials(podSpec, secret)

			// Check environment variable is set
			Expect(podSpec.Containers[0].Env).To(ContainElement(corev1.EnvVar{
				Name:  "GOOGLE_APPLICATION_CREDENTIALS",
				Value: "/credentials/gcs.json",
			}))

			// Check volume mount exists
			Expect(podSpec.Containers[0].VolumeMounts).To(ContainElement(corev1.VolumeMount{
				Name:      "credentials",
				MountPath: "/credentials",
				ReadOnly:  true,
			}))

			// Check volume exists with correct items
			Expect(podSpec.Volumes).To(HaveLen(1))
			Expect(podSpec.Volumes[0].Name).To(Equal("credentials"))
			Expect(podSpec.Volumes[0].VolumeSource.Secret).NotTo(BeNil())
			Expect(podSpec.Volumes[0].VolumeSource.Secret.SecretName).To(Equal("test-secret"))
			Expect(podSpec.Volumes[0].VolumeSource.Secret.Items).To(ContainElement(corev1.KeyToPath{
				Key:  "GOOGLE_APPLICATION_CREDENTIALS",
				Path: "gcs.json",
			}))
		})

		It("should configure SFTP credentials only", func() {
			secret.Data["SFTP_KEY_FILE"] = []byte("ssh-private-key-content")

			mover.configureCredentials(podSpec, secret)

			// Check environment variable is set
			Expect(podSpec.Containers[0].Env).To(ContainElement(corev1.EnvVar{
				Name:  "SFTP_KEY_FILE",
				Value: "/credentials/sftp_key",
			}))

			// Check volume mount exists
			Expect(podSpec.Containers[0].VolumeMounts).To(ContainElement(corev1.VolumeMount{
				Name:      "credentials",
				MountPath: "/credentials",
				ReadOnly:  true,
			}))

			// Check volume exists with correct items and SSH key permissions
			Expect(podSpec.Volumes).To(HaveLen(1))
			Expect(podSpec.Volumes[0].VolumeSource.Secret.Items).To(ContainElement(corev1.KeyToPath{
				Key:  "SFTP_KEY_FILE",
				Path: "sftp_key",
				Mode: ptr.To[int32](0600),
			}))
		})

		It("should configure multiple credentials without conflicts", func() {
			secret.Data["GOOGLE_APPLICATION_CREDENTIALS"] = []byte(`{"type": "service_account"}`)
			secret.Data["GOOGLE_DRIVE_CREDENTIALS"] = []byte(`{"type": "oauth2"}`)
			secret.Data["SFTP_KEY_FILE"] = []byte("ssh-private-key-content")

			mover.configureCredentials(podSpec, secret)

			// Check all environment variables are set
			Expect(podSpec.Containers[0].Env).To(ContainElement(corev1.EnvVar{
				Name:  "GOOGLE_APPLICATION_CREDENTIALS",
				Value: "/credentials/gcs.json",
			}))
			Expect(podSpec.Containers[0].Env).To(ContainElement(corev1.EnvVar{
				Name:  "GOOGLE_DRIVE_CREDENTIALS",
				Value: "/credentials/gdrive.json",
			}))
			Expect(podSpec.Containers[0].Env).To(ContainElement(corev1.EnvVar{
				Name:  "SFTP_KEY_FILE",
				Value: "/credentials/sftp_key",
			}))

			// Check only one volume mount exists (no conflicts)
			credentialMounts := 0
			for _, mount := range podSpec.Containers[0].VolumeMounts {
				if mount.MountPath == "/credentials" {
					credentialMounts++
				}
			}
			Expect(credentialMounts).To(Equal(1), "Should have exactly one credential mount")

			// Check only one volume exists with all credential files
			Expect(podSpec.Volumes).To(HaveLen(1))
			volume := podSpec.Volumes[0]
			Expect(volume.Name).To(Equal("credentials"))
			Expect(volume.VolumeSource.Secret.Items).To(HaveLen(3))
			Expect(volume.VolumeSource.Secret.Items).To(ContainElement(corev1.KeyToPath{
				Key:  "GOOGLE_APPLICATION_CREDENTIALS",
				Path: "gcs.json",
			}))
			Expect(volume.VolumeSource.Secret.Items).To(ContainElement(corev1.KeyToPath{
				Key:  "GOOGLE_DRIVE_CREDENTIALS",
				Path: "gdrive.json",
			}))
			Expect(volume.VolumeSource.Secret.Items).To(ContainElement(corev1.KeyToPath{
				Key:  "SFTP_KEY_FILE",
				Path: "sftp_key",
				Mode: ptr.To[int32](0600),
			}))
		})

		It("should not configure credentials when none exist", func() {
			// Empty secret data
			mover.configureCredentials(podSpec, secret)

			// Check no environment variables are added
			for _, env := range podSpec.Containers[0].Env {
				Expect(env.Name).NotTo(Equal("GOOGLE_APPLICATION_CREDENTIALS"))
				Expect(env.Name).NotTo(Equal("GOOGLE_DRIVE_CREDENTIALS"))
				Expect(env.Name).NotTo(Equal("SFTP_KEY_FILE"))
			}

			// Check no volume mounts are added
			for _, mount := range podSpec.Containers[0].VolumeMounts {
				Expect(mount.MountPath).NotTo(Equal("/credentials"))
			}

			// Check no volumes are added
			Expect(podSpec.Volumes).To(HaveLen(0))
		})

		It("should set proper file permissions", func() {
			secret.Data["GOOGLE_APPLICATION_CREDENTIALS"] = []byte(`{"type": "service_account"}`)
			secret.Data["SFTP_KEY_FILE"] = []byte("ssh-private-key-content")

			mover.configureCredentials(podSpec, secret)

			volume := podSpec.Volumes[0]
			// Check default mode is set to read-only for security
			Expect(volume.VolumeSource.Secret.DefaultMode).To(Equal(ptr.To[int32](0400)))

			// Check SSH key has more restrictive permissions
			for _, item := range volume.Secret.Items {
				if item.Path == "sftp_key" {
					Expect(item.Mode).To(Equal(ptr.To[int32](0600)))
				} else {
					Expect(item.Mode).To(BeNil()) // Uses default mode
				}
			}
		})
	})

	Context("Kopia environment variables", func() {
		var mover *Mover
		var secret *corev1.Secret

		BeforeEach(func() {
			mover = &Mover{
				username: "test-user",
				hostname: "test-host",
			}
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-secret",
				},
				Data: map[string][]byte{
					"KOPIA_REPOSITORY": []byte("s3://test-bucket/path"),
					"KOPIA_PASSWORD":   []byte("test-password"),
				},
			}
		})

		It("should include username and hostname overrides", func() {
			envVars := mover.buildEnvironmentVariables(secret)

			Expect(envVars).To(ContainElement(corev1.EnvVar{
				Name:  "KOPIA_OVERRIDE_USERNAME",
				Value: "test-user",
			}))
			Expect(envVars).To(ContainElement(corev1.EnvVar{
				Name:  "KOPIA_OVERRIDE_HOSTNAME",
				Value: "test-host",
			}))
		})

		It("should include all backend environment variables", func() {
			envVars := mover.buildEnvironmentVariables(secret)

			// Check mandatory variables
			found := false
			for _, env := range envVars {
				if env.Name == "KOPIA_REPOSITORY" {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "KOPIA_REPOSITORY should be present")

			// Check some backend-specific variables are included
			backendVars := []string{
				"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "KOPIA_S3_BUCKET",
				"AZURE_ACCOUNT_NAME", "AZURE_ACCOUNT_KEY", "KOPIA_AZURE_CONTAINER",
				"KOPIA_GCS_BUCKET", "GOOGLE_APPLICATION_CREDENTIALS",
				"KOPIA_B2_BUCKET", "B2_ACCOUNT_ID", "B2_APPLICATION_KEY",
				"WEBDAV_URL", "WEBDAV_USERNAME", "WEBDAV_PASSWORD",
				"SFTP_HOST", "SFTP_USERNAME", "SFTP_KEY_FILE",
				"RCLONE_REMOTE_PATH", "RCLONE_EXE", "RCLONE_CONFIG",
				"GOOGLE_DRIVE_FOLDER_ID", "GOOGLE_DRIVE_CREDENTIALS",
			}

			for _, varName := range backendVars {
				found := false
				for _, env := range envVars {
					if env.Name == varName {
						found = true
						break
					}
				}
				Expect(found).To(BeTrue(), "Backend variable %s should be present", varName)
			}
		})
	})

	Context("Kopia log filter", func() {
		It("should filter kopia log lines", func() {
			// Test lines that should be included
			testLines := []string{
				"Snapshot k123456 created",
				"Uploaded 1234 bytes",
				"Restored 100 files",
				"Successfully completed",
				"Connected to repository",
				"Repository opened",
				"Maintenance completed",
				"ERROR: something went wrong",
				"FATAL: critical error",
				"kopia completed in 30s",
			}

			for _, line := range testLines {
				result := LogLineFilterSuccess(line)
				Expect(result).NotTo(BeNil())
				Expect(*result).To(Equal(line))
			}

			// Test lines that should be filtered out
			filteredLines := []string{
				"Random debug message",
				"Verbose internal details",
				"Unimportant status update",
			}

			for _, line := range filteredLines {
				result := LogLineFilterSuccess(line)
				Expect(result).To(BeNil())
			}
		})
	})
})
