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
	"context"
	"flag"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/viper"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

var _ = Describe("Kopia Filesystem Destination", func() {
	var (
		ctx       = context.Background()
		namespace *corev1.Namespace
		rs        *volsyncv1alpha1.ReplicationSource
		destPVC   *corev1.PersistentVolumeClaim
		logger    = zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))
	)

	BeforeEach(func() {
		// Create test namespace
		namespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "kopia-fs-test-",
			},
		}
		Expect(k8sClient.Create(ctx, namespace)).To(Succeed())

		// Create destination PVC for filesystem backup
		destPVC = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "backup-pvc",
				Namespace: namespace.Name,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("1Gi"),
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, destPVC)).To(Succeed())

		// Mark PVC as bound for testing
		destPVC.Status.Phase = corev1.ClaimBound
		Expect(k8sClient.Status().Update(ctx, destPVC)).To(Succeed())

		// Create repository secret
		repoSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kopia-secret",
				Namespace: namespace.Name,
			},
			Data: map[string][]byte{
				"KOPIA_REPOSITORY": []byte("filesystem:"),
				"KOPIA_PASSWORD":   []byte("test-password"),
			},
		}
		Expect(k8sClient.Create(ctx, repoSecret)).To(Succeed())

		// Create source PVC
		sourcePVC := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "source-pvc",
				Namespace: namespace.Name,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("1Gi"),
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, sourcePVC)).To(Succeed())
		sourcePVC.Status.Phase = corev1.ClaimBound
		Expect(k8sClient.Status().Update(ctx, sourcePVC)).To(Succeed())

		// Create base ReplicationSource
		rs = &volsyncv1alpha1.ReplicationSource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-rs",
				Namespace: namespace.Name,
			},
			Spec: volsyncv1alpha1.ReplicationSourceSpec{
				SourcePVC: "source-pvc",
				Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
					Repository: "kopia-secret",
				},
			},
		}
	})

	AfterEach(func() {
		// Clean up namespace
		Expect(k8sClient.Delete(ctx, namespace)).To(Succeed())
	})

	Context("When filesystem destination is configured", func() {
		It("should configure the PVC mount correctly", func() {
			// Configure filesystem destination
			customPath := "my-backups"
			rs.Spec.Kopia.FilesystemDestination = &volsyncv1alpha1.FilesystemDestinationSpec{
				ClaimName: destPVC.Name,
				Path:      &customPath,
			}

			// Create the mover
			viper := viper.New()
			flags := flag.NewFlagSet("test", flag.ContinueOnError)
			b, err := newBuilder(viper, flags)
			Expect(err).NotTo(HaveOccurred())

			mover, err := b.FromSource(k8sClient, logger, &events.FakeRecorder{}, rs, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(mover).NotTo(BeNil())

			// Verify filesystem fields are set
			m, ok := mover.(*Mover)
			Expect(ok).To(BeTrue())
			Expect(m.filesystemDestination).NotTo(BeNil())
			Expect(m.filesystemDestination.ClaimName).To(Equal(destPVC.Name))
			Expect(m.filesystemRepoPath).To(Equal("/kopia/my-backups"))
		})

		It("should use default path when not specified", func() {
			// Configure filesystem destination without custom path
			rs.Spec.Kopia.FilesystemDestination = &volsyncv1alpha1.FilesystemDestinationSpec{
				ClaimName: destPVC.Name,
			}

			// Create the mover
			viper := viper.New()
			flags := flag.NewFlagSet("test", flag.ContinueOnError)
			b, err := newBuilder(viper, flags)
			Expect(err).NotTo(HaveOccurred())

			mover, err := b.FromSource(k8sClient, logger, &events.FakeRecorder{}, rs, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(mover).NotTo(BeNil())

			// Verify default path is used
			m, ok := mover.(*Mover)
			Expect(ok).To(BeTrue())
			Expect(m.filesystemRepoPath).To(Equal("/kopia/backups"))
		})

		It("should validate PVC existence", func() {
			// Configure filesystem destination with non-existent PVC
			rs.Spec.Kopia.FilesystemDestination = &volsyncv1alpha1.FilesystemDestinationSpec{
				ClaimName: "non-existent-pvc",
			}

			// Create the mover
			viper := viper.New()
			flags := flag.NewFlagSet("test", flag.ContinueOnError)
			b, err := newBuilder(viper, flags)
			Expect(err).NotTo(HaveOccurred())

			mover, err := b.FromSource(k8sClient, logger, &events.FakeRecorder{}, rs, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(mover).NotTo(BeNil())

			// Validate should fail
			m, ok := mover.(*Mover)
			Expect(ok).To(BeTrue())
			err = m.validateFilesystemDestination(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})

		It("should validate PVC is bound", func() {
			// Create unbound PVC
			unboundPVC := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "unbound-pvc",
					Namespace: namespace.Name,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("1Gi"),
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, unboundPVC)).To(Succeed())

			// Configure filesystem destination with unbound PVC
			rs.Spec.Kopia.FilesystemDestination = &volsyncv1alpha1.FilesystemDestinationSpec{
				ClaimName: unboundPVC.Name,
			}

			// Create the mover
			viper := viper.New()
			flags := flag.NewFlagSet("test", flag.ContinueOnError)
			b, err := newBuilder(viper, flags)
			Expect(err).NotTo(HaveOccurred())

			mover, err := b.FromSource(k8sClient, logger, &events.FakeRecorder{}, rs, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(mover).NotTo(BeNil())

			// Validate should fail
			m, ok := mover.(*Mover)
			Expect(ok).To(BeTrue())
			err = m.validateFilesystemDestination(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not bound"))
		})

		It("should add KOPIA_FS_PATH environment variable", func() {
			// Configure filesystem destination
			rs.Spec.Kopia.FilesystemDestination = &volsyncv1alpha1.FilesystemDestinationSpec{
				ClaimName: destPVC.Name,
			}

			// Create the mover
			viper := viper.New()
			flags := flag.NewFlagSet("test", flag.ContinueOnError)
			b, err := newBuilder(viper, flags)
			Expect(err).NotTo(HaveOccurred())

			mover, err := b.FromSource(k8sClient, logger, &events.FakeRecorder{}, rs, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(mover).NotTo(BeNil())

			// Build environment variables
			m, ok := mover.(*Mover)
			Expect(ok).To(BeTrue())
			repoSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kopia-secret",
					Namespace: namespace.Name,
				},
				Data: map[string][]byte{
					"KOPIA_REPOSITORY": []byte("filesystem:"),
					"KOPIA_PASSWORD":   []byte("test-password"),
				},
			}

			envVars := m.buildEnvironmentVariables(repoSecret)

			// Check for KOPIA_FS_PATH
			var foundFSPath bool
			for _, env := range envVars {
				if env.Name == "KOPIA_FS_PATH" {
					foundFSPath = true
					Expect(env.Value).To(Equal("/kopia/backups"))
					break
				}
			}
			Expect(foundFSPath).To(BeTrue(), "KOPIA_FS_PATH should be set")
		})

		It("should configure pod spec with filesystem volume", func() {
			// Configure filesystem destination
			rs.Spec.Kopia.FilesystemDestination = &volsyncv1alpha1.FilesystemDestinationSpec{
				ClaimName: destPVC.Name,
				ReadOnly:  true,
			}

			// Create the mover
			viper := viper.New()
			flags := flag.NewFlagSet("test", flag.ContinueOnError)
			b, err := newBuilder(viper, flags)
			Expect(err).NotTo(HaveOccurred())

			mover, err := b.FromSource(k8sClient, logger, &events.FakeRecorder{}, rs, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(mover).NotTo(BeNil())

			// Create a pod spec to test configuration
			job := &batchv1.Job{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "kopia",
								},
							},
						},
					},
				},
			}

			// Configure filesystem destination on pod
			m, ok := mover.(*Mover)
			Expect(ok).To(BeTrue())
			err = m.configureFilesystemDestination(&job.Spec.Template.Spec)
			Expect(err).NotTo(HaveOccurred())

			// Verify volume is added
			foundVolume := false
			for _, vol := range job.Spec.Template.Spec.Volumes {
				if vol.Name == "filesystem-destination" {
					foundVolume = true
					Expect(vol.PersistentVolumeClaim).NotTo(BeNil())
					Expect(vol.PersistentVolumeClaim.ClaimName).To(Equal(destPVC.Name))
					Expect(vol.PersistentVolumeClaim.ReadOnly).To(BeTrue())
					break
				}
			}
			Expect(foundVolume).To(BeTrue(), "filesystem-destination volume should be added")

			// Verify volume mount is added
			foundMount := false
			for _, mount := range job.Spec.Template.Spec.Containers[0].VolumeMounts {
				if mount.Name == "filesystem-destination" {
					foundMount = true
					Expect(mount.MountPath).To(Equal("/kopia"))
					Expect(mount.ReadOnly).To(BeTrue())
					break
				}
			}
			Expect(foundMount).To(BeTrue(), "filesystem-destination volume mount should be added")
		})
	})

	Context("Path validation and sanitization", func() {
		It("should sanitize paths with parent directory references", func() {
			// Test path traversal prevention
			maliciousPath := "../../../etc/passwd"
			rs.Spec.Kopia.FilesystemDestination = &volsyncv1alpha1.FilesystemDestinationSpec{
				ClaimName: destPVC.Name,
				Path:      &maliciousPath,
			}

			viper := viper.New()
			flags := flag.NewFlagSet("test", flag.ContinueOnError)
			b, err := newBuilder(viper, flags)
			Expect(err).NotTo(HaveOccurred())

			mover, err := b.FromSource(k8sClient, logger, &events.FakeRecorder{}, rs, false)
			Expect(err).NotTo(HaveOccurred())

			// Path should be sanitized to remove parent directory references
			m, ok := mover.(*Mover)
			Expect(ok).To(BeTrue())
			Expect(m.filesystemRepoPath).To(Equal("/kopia/etc/passwd"))
		})

		It("should sanitize absolute paths", func() {
			// Test absolute path handling
			absolutePath := "/absolute/path"
			rs.Spec.Kopia.FilesystemDestination = &volsyncv1alpha1.FilesystemDestinationSpec{
				ClaimName: destPVC.Name,
				Path:      &absolutePath,
			}

			viper := viper.New()
			flags := flag.NewFlagSet("test", flag.ContinueOnError)
			b, err := newBuilder(viper, flags)
			Expect(err).NotTo(HaveOccurred())

			mover, err := b.FromSource(k8sClient, logger, &events.FakeRecorder{}, rs, false)
			Expect(err).NotTo(HaveOccurred())

			// Absolute path should be converted to relative
			m, ok := mover.(*Mover)
			Expect(ok).To(BeTrue())
			Expect(m.filesystemRepoPath).To(Equal("/kopia/absolute/path"))
		})

		It("should handle empty path after sanitization", func() {
			// Test that empty/invalid paths fall back to default
			invalidPath := "../../../"
			rs.Spec.Kopia.FilesystemDestination = &volsyncv1alpha1.FilesystemDestinationSpec{
				ClaimName: destPVC.Name,
				Path:      &invalidPath,
			}

			viper := viper.New()
			flags := flag.NewFlagSet("test", flag.ContinueOnError)
			b, err := newBuilder(viper, flags)
			Expect(err).NotTo(HaveOccurred())

			mover, err := b.FromSource(k8sClient, logger, &events.FakeRecorder{}, rs, false)
			Expect(err).NotTo(HaveOccurred())

			// Should fall back to default "backups" path
			m, ok := mover.(*Mover)
			Expect(ok).To(BeTrue())
			Expect(m.filesystemRepoPath).To(Equal("/kopia/backups"))
		})

		It("should accept valid relative paths", func() {
			validPath := "relative/path"
			rs.Spec.Kopia.FilesystemDestination = &volsyncv1alpha1.FilesystemDestinationSpec{
				ClaimName: destPVC.Name,
				Path:      &validPath,
			}

			viper := viper.New()
			flags := flag.NewFlagSet("test", flag.ContinueOnError)
			b, err := newBuilder(viper, flags)
			Expect(err).NotTo(HaveOccurred())

			mover, err := b.FromSource(k8sClient, logger, &events.FakeRecorder{}, rs, false)
			Expect(err).NotTo(HaveOccurred())

			m, ok := mover.(*Mover)
			Expect(ok).To(BeTrue())
			Expect(m.filesystemRepoPath).To(Equal("/kopia/relative/path"))
		})

		It("should handle mount path conflicts", func() {
			rs.Spec.Kopia.FilesystemDestination = &volsyncv1alpha1.FilesystemDestinationSpec{
				ClaimName: destPVC.Name,
			}

			viper := viper.New()
			flags := flag.NewFlagSet("test", flag.ContinueOnError)
			b, err := newBuilder(viper, flags)
			Expect(err).NotTo(HaveOccurred())

			mover, err := b.FromSource(k8sClient, logger, &events.FakeRecorder{}, rs, false)
			Expect(err).NotTo(HaveOccurred())

			// Create a pod spec with an existing mount at /kopia
			job := &batchv1.Job{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "kopia",
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "existing-volume",
											MountPath: "/kopia",
										},
									},
								},
							},
						},
					},
				},
			}

			// Try to configure filesystem destination - should fail due to conflict
			m, ok := mover.(*Mover)
			Expect(ok).To(BeTrue())
			err = m.configureFilesystemDestination(&job.Spec.Template.Spec)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("mount path /kopia is already in use"))
		})
	})
})
