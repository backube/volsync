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

const (
	// kopiaRepositoryEnvVar is the environment variable name for Kopia repository configuration
	kopiaRepositoryEnvVar = "KOPIA_REPOSITORY"
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
				kopiaRepositoryEnvVar: []byte("filesystem:"),
				"KOPIA_PASSWORD":      []byte("test-password"),
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

	Context("When repository PVC is configured", func() {
		It("should configure the PVC mount correctly", func() {
			// Configure repository PVC
			rs.Spec.Kopia.RepositoryPVC = &destPVC.Name

			// Create the mover
			viper := viper.New()
			flags := flag.NewFlagSet("test", flag.ContinueOnError)
			b, err := newBuilder(viper, flags)
			Expect(err).NotTo(HaveOccurred())

			mover, err := b.FromSource(k8sClient, logger, &events.FakeRecorder{}, rs, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(mover).NotTo(BeNil())

			// Verify repository PVC field is set
			m, ok := mover.(*Mover)
			Expect(ok).To(BeTrue())
			Expect(m.repositoryPVC).NotTo(BeNil())
			Expect(*m.repositoryPVC).To(Equal(destPVC.Name))
		})

		It("should validate PVC existence", func() {
			// Configure repository PVC with non-existent PVC
			nonExistentPVC := "non-existent-pvc"
			rs.Spec.Kopia.RepositoryPVC = &nonExistentPVC

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
			err = m.validateRepositoryPVC(ctx)
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

			// Configure repository PVC with unbound PVC
			rs.Spec.Kopia.RepositoryPVC = &unboundPVC.Name

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
			err = m.validateRepositoryPVC(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not bound"))
		})

		It("should set "+kopiaRepositoryEnvVar+" to filesystem URL when using repository PVC", func() {
			// Configure repository PVC
			rs.Spec.Kopia.RepositoryPVC = &destPVC.Name

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
					kopiaRepositoryEnvVar: []byte("filesystem:"),
					"KOPIA_PASSWORD":      []byte("test-password"),
				},
			}

			envVars := m.buildEnvironmentVariables(repoSecret)

			// Check for kopiaRepositoryEnvVar with filesystem URL
			var foundRepositoryURL bool
			for _, env := range envVars {
				if env.Name == kopiaRepositoryEnvVar {
					foundRepositoryURL = true
					Expect(env.Value).To(Equal("filesystem:///kopia/repository"))
					break
				}
			}
			Expect(foundRepositoryURL).To(BeTrue(), kopiaRepositoryEnvVar+" should be set to filesystem URL")
		})

		It("should configure pod spec with repository PVC volume", func() {
			// Configure repository PVC
			rs.Spec.Kopia.RepositoryPVC = &destPVC.Name

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

			// Configure repository PVC on pod
			m, ok := mover.(*Mover)
			Expect(ok).To(BeTrue())
			err = m.configureRepositoryPVC(&job.Spec.Template.Spec)
			Expect(err).NotTo(HaveOccurred())

			// Verify volume is added
			foundVolume := false
			for _, vol := range job.Spec.Template.Spec.Volumes {
				if vol.Name == "repository-pvc" {
					foundVolume = true
					Expect(vol.PersistentVolumeClaim).NotTo(BeNil())
					Expect(vol.PersistentVolumeClaim.ClaimName).To(Equal(destPVC.Name))
					Expect(vol.PersistentVolumeClaim.ReadOnly).To(BeFalse())
					break
				}
			}
			Expect(foundVolume).To(BeTrue(), "repository-pvc volume should be added")

			// Verify volume mount is added
			foundMount := false
			for _, mount := range job.Spec.Template.Spec.Containers[0].VolumeMounts {
				if mount.Name == "repository-pvc" {
					foundMount = true
					Expect(mount.MountPath).To(Equal("/kopia"))
					Expect(mount.ReadOnly).To(BeFalse())
					break
				}
			}
			Expect(foundMount).To(BeTrue(), "repository-pvc volume mount should be added")
		})
	})

	Context("Fixed repository path", func() {
		It("should always use /kopia/repository as the repository path", func() {
			// Configure repository PVC
			rs.Spec.Kopia.RepositoryPVC = &destPVC.Name

			viper := viper.New()
			flags := flag.NewFlagSet("test", flag.ContinueOnError)
			b, err := newBuilder(viper, flags)
			Expect(err).NotTo(HaveOccurred())

			mover, err := b.FromSource(k8sClient, logger, &events.FakeRecorder{}, rs, false)
			Expect(err).NotTo(HaveOccurred())

			// Verify fixed path is always used
			m, ok := mover.(*Mover)
			Expect(ok).To(BeTrue())

			// Build environment variables to check kopiaRepositoryEnvVar
			repoSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kopia-secret",
					Namespace: namespace.Name,
				},
				Data: map[string][]byte{
					kopiaRepositoryEnvVar: []byte("filesystem:"),
					"KOPIA_PASSWORD":      []byte("test-password"),
				},
			}

			envVars := m.buildEnvironmentVariables(repoSecret)

			// Find kopiaRepositoryEnvVar environment variable
			var foundPath string
			for _, env := range envVars {
				if env.Name == kopiaRepositoryEnvVar {
					foundPath = env.Value
					break
				}
			}

			// Should be filesystem URL pointing to /kopia/repository
			Expect(foundPath).To(Equal("filesystem:///kopia/repository"))
		})

		It("should handle mount path conflicts", func() {
			rs.Spec.Kopia.RepositoryPVC = &destPVC.Name

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

			// Try to configure repository PVC - should fail due to conflict
			m, ok := mover.(*Mover)
			Expect(ok).To(BeTrue())
			err = m.configureRepositoryPVC(&job.Spec.Template.Spec)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("mount path /kopia is already in use"))
		})
	})
})
