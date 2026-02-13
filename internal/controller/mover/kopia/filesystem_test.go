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

	Context("When moverVolumes with PVC is configured", func() {
		It("should detect repository from moverVolumes", func() {
			// Configure moverVolumes with a PVC
			rs.Spec.Kopia.MoverVolumes = []volsyncv1alpha1.MoverVolume{
				{
					MountPath: "kopia-repo",
					VolumeSource: volsyncv1alpha1.MoverVolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: destPVC.Name,
						},
					},
				},
			}

			// Create the mover
			viper := viper.New()
			flags := flag.NewFlagSet("test", flag.ContinueOnError)
			b, err := newBuilder(viper, flags)
			Expect(err).NotTo(HaveOccurred())

			mover, err := b.FromSource(k8sClient, logger, &events.FakeRecorder{}, rs, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(mover).NotTo(BeNil())

			// Verify moverVolumes field is set
			m, ok := mover.(*Mover)
			Expect(ok).To(BeTrue())
			Expect(len(m.moverConfig.MoverVolumes)).To(Equal(1))
			Expect(m.moverConfig.MoverVolumes[0].MountPath).To(Equal("kopia-repo"))
		})

		It("should validate PVC existence in moverVolumes", func() {
			// Configure moverVolumes with non-existent PVC
			rs.Spec.Kopia.MoverVolumes = []volsyncv1alpha1.MoverVolume{
				{
					MountPath: "kopia-repo",
					VolumeSource: volsyncv1alpha1.MoverVolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "non-existent-pvc",
						},
					},
				},
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
			_, err = m.validateRepository(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})

		It("should set "+kopiaRepositoryEnvVar+" to filesystem URL when using moverVolumes PVC", func() {
			// Configure moverVolumes with a PVC
			rs.Spec.Kopia.MoverVolumes = []volsyncv1alpha1.MoverVolume{
				{
					MountPath: "kopia-repo",
					VolumeSource: volsyncv1alpha1.MoverVolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: destPVC.Name,
						},
					},
				},
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
					Expect(env.Value).To(Equal("filesystem:///mnt/kopia-repo"))
					break
				}
			}
			Expect(foundRepositoryURL).To(BeTrue(), kopiaRepositoryEnvVar+" should be set to filesystem URL")
		})

		It("should use remote repository when no PVC in moverVolumes", func() {
			// Configure moverVolumes with only a Secret (no PVC)
			rs.Spec.Kopia.MoverVolumes = []volsyncv1alpha1.MoverVolume{
				{
					MountPath: "my-secret",
					VolumeSource: volsyncv1alpha1.MoverVolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: "some-secret",
						},
					},
				},
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
					kopiaRepositoryEnvVar: []byte("s3://my-bucket/path"),
					"KOPIA_PASSWORD":      []byte("test-password"),
				},
			}

			envVars := m.buildEnvironmentVariables(repoSecret)

			// Should use repository from secret (not filesystem)
			var foundEnvFrom bool
			for _, env := range envVars {
				if env.Name == kopiaRepositoryEnvVar {
					// Should be from secret, not a direct value
					if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
						foundEnvFrom = true
						Expect(env.ValueFrom.SecretKeyRef.Name).To(Equal("kopia-secret"))
						Expect(env.ValueFrom.SecretKeyRef.Key).To(Equal(kopiaRepositoryEnvVar))
					}
					break
				}
			}
			Expect(foundEnvFrom).To(BeTrue(), kopiaRepositoryEnvVar+" should be from secret")
		})
	})
})
