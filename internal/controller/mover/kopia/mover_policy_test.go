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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

var _ = Describe("Kopia Policy Configuration", func() {
	var (
		ctx       context.Context
		k8sClient client.Client
		mover     *Mover
		namespace string
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "test-namespace"

		// Create a fake client
		k8sClient = fake.NewClientBuilder().Build()

		// Create a basic mover instance
		mover = &Mover{
			client: k8sClient,
			logger: zap.New(zap.UseDevMode(true)),
			owner: &volsyncv1alpha1.ReplicationSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-source",
					Namespace: namespace,
				},
			},
		}
	})

	Describe("validatePolicyConfig", func() {
		Context("when no policy config is specified", func() {
			It("should return nil", func() {
				mover.policyConfig = nil
				obj, err := mover.validatePolicyConfig(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj).To(BeNil())
			})
		})

		Context("when using ConfigMap for policy", func() {
			BeforeEach(func() {
				// Create a ConfigMap
				cm := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "policy-config",
						Namespace: namespace,
					},
					Data: map[string]string{
						"global-policy.json": `{"retention": {"keepDaily": 7}}`,
						"repository.config":  `{"enableActions": false}`,
					},
				}
				Expect(k8sClient.Create(ctx, cm)).To(Succeed())

				mover.policyConfig = &volsyncv1alpha1.KopiaPolicySpec{
					ConfigMapName: "policy-config",
				}
			})

			It("should validate successfully", func() {
				obj, err := mover.validatePolicyConfig(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj).NotTo(BeNil())
			})
		})

		Context("when using Secret for policy", func() {
			BeforeEach(func() {
				// Create a Secret
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "policy-secret",
						Namespace: namespace,
					},
					Data: map[string][]byte{
						"global-policy.json": []byte(`{"retention": {"keepDaily": 7}}`),
						"repository.config":  []byte(`{"enableActions": false}`),
					},
				}
				Expect(k8sClient.Create(ctx, secret)).To(Succeed())

				mover.policyConfig = &volsyncv1alpha1.KopiaPolicySpec{
					SecretName: "policy-secret",
				}
			})

			It("should validate successfully", func() {
				obj, err := mover.validatePolicyConfig(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj).NotTo(BeNil())
			})
		})

		Context("when both ConfigMap and Secret are specified", func() {
			BeforeEach(func() {
				mover.policyConfig = &volsyncv1alpha1.KopiaPolicySpec{
					ConfigMapName: "policy-config",
					SecretName:    "policy-secret",
				}
			})

			It("should return an error", func() {
				obj, err := mover.validatePolicyConfig(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("only one of secretName or configMapName"))
				Expect(obj).To(BeNil())
			})
		})

		Context("when ConfigMap doesn't exist", func() {
			BeforeEach(func() {
				mover.policyConfig = &volsyncv1alpha1.KopiaPolicySpec{
					ConfigMapName: "non-existent-config",
				}
			})

			It("should return an error", func() {
				obj, err := mover.validatePolicyConfig(ctx)
				Expect(err).To(HaveOccurred())
				Expect(obj).To(BeNil())
			})
		})

		Context("when using custom filenames", func() {
			BeforeEach(func() {
				// Create a ConfigMap with custom filenames
				cm := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "custom-policy-config",
						Namespace: namespace,
					},
					Data: map[string]string{
						"my-policy.json": `{"retention": {"keepDaily": 14}}`,
						"my-repo.json":   `{"uploadSpeed": 1048576}`,
					},
				}
				Expect(k8sClient.Create(ctx, cm)).To(Succeed())

				mover.policyConfig = &volsyncv1alpha1.KopiaPolicySpec{
					ConfigMapName:            "custom-policy-config",
					GlobalPolicyFilename:     "my-policy.json",
					RepositoryConfigFilename: "my-repo.json",
				}
			})

			It("should validate successfully with custom filenames", func() {
				obj, err := mover.validatePolicyConfig(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj).NotTo(BeNil())
			})
		})

		Context("when repositoryConfig contains invalid JSON", func() {
			BeforeEach(func() {
				mover.policyConfig = &volsyncv1alpha1.KopiaPolicySpec{
					RepositoryConfig: ptr.To("invalid json {"),
				}
			})

			It("should return an error", func() {
				obj, err := mover.validatePolicyConfig(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid JSON"))
				Expect(obj).To(BeNil())
			})
		})

		Context("when repositoryConfig contains valid JSON", func() {
			BeforeEach(func() {
				mover.policyConfig = &volsyncv1alpha1.KopiaPolicySpec{
					RepositoryConfig: ptr.To(`{"enableActions": true, "uploadSpeed": 5242880}`),
				}
			})

			It("should validate successfully", func() {
				obj, err := mover.validatePolicyConfig(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj).To(BeNil()) // No file-based config, so obj is nil
			})
		})
	})

	Describe("configureFilePolicyConfig", func() {
		var podSpec *corev1.PodSpec

		BeforeEach(func() {
			podSpec = &corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "kopia",
					},
				},
			}
		})

		Context("when using default filenames", func() {
			It("should configure environment variables with default paths", func() {
				mover.policyConfig = &volsyncv1alpha1.KopiaPolicySpec{
					ConfigMapName: "policy-config",
				}

				// Create a mock policy config object
				mockConfigObj := &mockCustomCAObject{
					volumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "policy-config",
							},
						},
					},
				}

				mover.configureFilePolicyConfig(podSpec, mockConfigObj)

				// Check environment variables
				envVars := podSpec.Containers[0].Env
				Expect(envVars).To(ContainElement(corev1.EnvVar{
					Name:  "KOPIA_CONFIG_PATH",
					Value: "/kopia-config",
				}))
				Expect(envVars).To(ContainElement(corev1.EnvVar{
					Name:  "KOPIA_GLOBAL_POLICY_FILE",
					Value: "/kopia-config/global-policy.json",
				}))
				Expect(envVars).To(ContainElement(corev1.EnvVar{
					Name:  "KOPIA_REPOSITORY_CONFIG_FILE",
					Value: "/kopia-config/repository.config",
				}))

				// Check volume mount
				Expect(podSpec.Containers[0].VolumeMounts).To(ContainElement(
					corev1.VolumeMount{
						Name:      "kopia-config",
						MountPath: "/kopia-config",
						ReadOnly:  true,
					},
				))

				// Check volume
				Expect(podSpec.Volumes).To(HaveLen(1))
				Expect(podSpec.Volumes[0].Name).To(Equal("kopia-config"))
				Expect(podSpec.Volumes[0].ConfigMap).NotTo(BeNil())
				Expect(podSpec.Volumes[0].ConfigMap.DefaultMode).To(Equal(ptr.To[int32](0644)))
			})
		})

		Context("when using custom filenames", func() {
			It("should configure environment variables with custom paths", func() {
				mover.policyConfig = &volsyncv1alpha1.KopiaPolicySpec{
					SecretName:               "policy-secret",
					GlobalPolicyFilename:     "custom-policy.json",
					RepositoryConfigFilename: "custom-repo.json",
				}

				// Create a mock policy config object for Secret
				mockConfigObj := &mockCustomCAObject{
					volumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: "policy-secret",
						},
					},
				}

				mover.configureFilePolicyConfig(podSpec, mockConfigObj)

				// Check environment variables
				envVars := podSpec.Containers[0].Env
				Expect(envVars).To(ContainElement(corev1.EnvVar{
					Name:  "KOPIA_GLOBAL_POLICY_FILE",
					Value: "/kopia-config/custom-policy.json",
				}))
				Expect(envVars).To(ContainElement(corev1.EnvVar{
					Name:  "KOPIA_REPOSITORY_CONFIG_FILE",
					Value: "/kopia-config/custom-repo.json",
				}))

				// Check volume uses Secret
				Expect(podSpec.Volumes).To(HaveLen(1))
				Expect(podSpec.Volumes[0].Secret).NotTo(BeNil())
				Expect(podSpec.Volumes[0].Secret.DefaultMode).To(Equal(ptr.To[int32](0644)))
			})
		})
	})
})

// Mock implementation of CustomCAObject for testing
type mockCustomCAObject struct {
	volumeSource corev1.VolumeSource
}

func (m *mockCustomCAObject) GetVolumeSource(_ string) corev1.VolumeSource {
	return m.volumeSource
}
