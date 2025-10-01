/*
Copyright 2022 The VolSync authors.

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

package utils_test

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/internal/controller/utils"
)

var _ = Describe("utils tests", func() {
	Describe("PvcIsReadOnly", func() {
		var pvc *corev1.PersistentVolumeClaim

		storageClassName := "mytest-storage-class"

		BeforeEach(func() {
			pvc = &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc-1",
					Namespace: "test-pvc-1-namespace",
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: &storageClassName,
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
				},
			}
		})

		When("PVC accessModes is set to only ROX", func() {
			BeforeEach(func() {
				pvc.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadOnlyMany}
			})

			When("pvc.status.accessmodes is not defined", func() {
				It("Should determine the pvc is read-only from the pvc spec", func() {
					Expect(utils.PvcIsReadOnly(pvc)).To(BeTrue())
				})
			})

			When("pvc.status.accessmodes is defined", func() {
				BeforeEach(func() {
					pvc.Status.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadOnlyMany}

					// Clear out the spec just to ensure the code gets the value from the status first
					pvc.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{}
				})

				It("Should determine the pvc is read-only from the pvc status", func() {
					Expect(utils.PvcIsReadOnly(pvc)).To(BeTrue())
				})
			})
		})

		When("PVC access modes contains any writable access mode", func() {
			BeforeEach(func() {
				pvc.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteMany,
					corev1.ReadOnlyMany, // Even if ROX is here, we should return readOnly is false
				}
			})

			When("pvc.status.accessmodes is not defined", func() {
				It("Should determine the pvc is not read-only from the pvc spec", func() {
					Expect(utils.PvcIsReadOnly(pvc)).To(BeFalse())
				})
			})

			When("pvc.status.accessmodes is defined", func() {
				BeforeEach(func() {
					pvc.Status.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
				})

				It("Should determine the pvc is not read-only from the pvc status", func() {
					Expect(utils.PvcIsReadOnly(pvc)).To(BeFalse())
				})
			})
		})
	})

	Describe("PvcIsBlockMode", func() {
		var pvc *corev1.PersistentVolumeClaim

		storageClassName := "mytest-storage-class"

		BeforeEach(func() {
			pvc = &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc-1",
					Namespace: "test-pvc-1-namespace",
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: &storageClassName,
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
				},
			}
		})

		When("The PVC does not have VolumeMode set", func() {
			It("Should not be blockmode", func() {
				Expect(utils.PvcIsBlockMode(pvc)).To(BeFalse())
			})
		})
		When("The PVC has volumemode filesystem", func() {
			It("Should not be blockmode", func() {
				volumeMode := corev1.PersistentVolumeFilesystem
				pvc.Spec.VolumeMode = &volumeMode
				Expect(utils.PvcIsBlockMode(pvc)).To(BeFalse())
			})
		})
		When("The PVC has volumemode block", func() {
			It("Should be blockmode", func() {
				volumeMode := corev1.PersistentVolumeBlock
				pvc.Spec.VolumeMode = &volumeMode
				Expect(utils.PvcIsBlockMode(pvc)).To(BeTrue())
			})
		})
	})

	Describe("AppendEnvVarsForClusterWideProxy", func() {
		envVarsOrig := []corev1.EnvVar{
			{
				Name:  "existingvar1",
				Value: "value1",
			},
			{
				Name:  "EXISTINGVAR2",
				Value: "VALUE2",
			},
		}

		var envVars []corev1.EnvVar

		BeforeEach(func() {
			// Reset envVars back to initial starting value for test
			envVars = make([]corev1.EnvVar, len(envVarsOrig))
			copy(envVars, envVarsOrig)
		})

		AfterEach(func() {
			os.Unsetenv("HTTP_PROXY")
			os.Unsetenv("HTTPS_PROXY")
			os.Unsetenv("NO_PROXY")
		})

		When("no proxy env vars are set", func() {
			It("Should not modify the existing env vars", func() {
				envVars = utils.AppendEnvVarsForClusterWideProxy(envVars)
				Expect(envVars).To(Equal(envVarsOrig))
			})
		})

		When("proxy env vars are set", func() {
			httpProxyValue := "http://myproxy.com"
			httpsProxyValue := "https://myproxy-secure.com"
			noProxyValue := "myinternal.network.acmewidgets.com,testing.acmewidgets.com"

			BeforeEach(func() {
				os.Setenv("HTTP_PROXY", httpProxyValue)
				os.Setenv("HTTPS_PROXY", httpsProxyValue)
				os.Setenv("NO_PROXY", noProxyValue)
			})

			It("Should set the appropriate env vars", func() {
				envVars = utils.AppendEnvVarsForClusterWideProxy(envVars)
				for i := range envVarsOrig {
					// Original env vars should still be set
					origVar := envVarsOrig[i]
					Expect(envVars).To(ContainElements(origVar))
				}

				// Proxy env vars should be set
				Expect(envVars).To(ContainElement(corev1.EnvVar{
					Name:  "HTTP_PROXY",
					Value: httpProxyValue,
				}))
				Expect(envVars).To(ContainElement(corev1.EnvVar{
					Name:  "http_proxy",
					Value: httpProxyValue,
				}))
				Expect(envVars).To(ContainElement(corev1.EnvVar{
					Name:  "HTTPS_PROXY",
					Value: httpsProxyValue,
				}))
				Expect(envVars).To(ContainElement(corev1.EnvVar{
					Name:  "https_proxy",
					Value: httpsProxyValue,
				}))
				Expect(envVars).To(ContainElement(corev1.EnvVar{
					Name:  "NO_PROXY",
					Value: noProxyValue,
				}))
				Expect(envVars).To(ContainElement(corev1.EnvVar{
					Name:  "no_proxy",
					Value: noProxyValue,
				}))
			})
		})
	})

	Describe("AppendRCloneEnvVars", func() {
		envVarsOrig := []corev1.EnvVar{
			{
				Name:  "existingvar1",
				Value: "value1",
			},
			{
				Name:  "EXISTINGVAR2",
				Value: "VALUE2",
			},
		}

		var envVars []corev1.EnvVar
		var secret *corev1.Secret

		BeforeEach(func() {
			// Reset envVars back to initial starting value for test
			envVars = make([]corev1.EnvVar, len(envVarsOrig))
			copy(envVars, envVarsOrig)

			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret-withrclonevars",
					Namespace: "test-secret-namespace",
				},
				Data: map[string][]byte{},
			}
		})

		When("no secret env vars are set", func() {
			It("Should not modify the existing env vars", func() {
				envVars = utils.AppendRCloneEnvVars(secret, envVars)
				Expect(envVars).To(Equal(envVarsOrig))
			})
		})

		When("no env vars containing RCLONE_ are set in the secret", func() {
			BeforeEach(func() {
				secret.Data = map[string][]byte{
					"testkey":        []byte("pineapples"),
					"NOT_RCLONE_VAR": []byte("kiwis"),
					"OTHER_VAR":      []byte("oranges"),
				}
			})
			It("Should not modify the existing env vars", func() {
				envVars = utils.AppendRCloneEnvVars(secret, envVars)
				Expect(envVars).To(Equal(envVarsOrig))
			})
		})

		When("RCLONE_ env vars are set", func() {
			BeforeEach(func() {
				secret.Data = map[string][]byte{
					"RCLONE_TESTVAR1": []byte("veryimportant"),
					"RCLONE_TESTVAR2": []byte("evenmoreimportant"),
					"NOT_RCLONE_VAR":  []byte("shouldntbeset"),
					"RCLONE_BWLIMIT":  []byte("5M:10M"),
				}
			})

			It("Should set the appropriate env vars", func() {
				envVars = utils.AppendRCloneEnvVars(secret, envVars)
				for i := range envVarsOrig {
					// Original env vars should still be set
					origVar := envVarsOrig[i]
					Expect(envVars).To(ContainElements(origVar))
				}

				t := true

				// RCLONE_ env vars from secret should be set
				Expect(envVars).To(ContainElement(corev1.EnvVar{
					Name: "RCLONE_TESTVAR1",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: secret.GetName(),
							},
							Key:      "RCLONE_TESTVAR1",
							Optional: &t,
						},
					},
				}))
				Expect(envVars).To(ContainElement(corev1.EnvVar{
					Name: "RCLONE_TESTVAR2",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: secret.GetName(),
							},
							Key:      "RCLONE_TESTVAR2",
							Optional: &t,
						},
					},
				}))
				Expect(envVars).To(ContainElement(corev1.EnvVar{
					Name: "RCLONE_BWLIMIT",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: secret.GetName(),
							},
							Key:      "RCLONE_BWLIMIT",
							Optional: &t,
						},
					},
				}))
			})
		})
	})

	Describe("UpdatePodTemplateSpecFromMoverConfig", func() {
		When("no pod template spec", func() {
			It("should not fail", func() {
				utils.UpdatePodTemplateSpecFromMoverConfig(nil, volsyncv1alpha1.MoverConfig{}, corev1.ResourceRequirements{})
			})
		})

		var podTemplateSpec *corev1.PodTemplateSpec
		existingLabelsOrig := map[string]string{
			"existingl": "12345",
		}

		BeforeEach(func() {
			// Copy the orig map so we keep orig unchanged to compare later
			existingLabels := map[string]string{}
			for k, v := range existingLabelsOrig {
				existingLabels[k] = v
			}

			podTemplateSpec = &corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-podtemplatespec",
					Namespace: "test-ns",
					Labels:    existingLabels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						// 1 container for this test - all movers currently using
						// only 1 container
						{
							Name:    "testcontainer0",
							Image:   "quay.io/testtesttest/test:test",
							Command: []string{"takeovertheworld.sh", "now"},
						},
					},
				},
			}
		})

		When("moverConfig does not have any moverResource requirements or pod labels", func() {
			var defaultResourceRequirements corev1.ResourceRequirements

			BeforeEach(func() {
				defaultResourceRequirements = corev1.ResourceRequirements{}
			})

			JustBeforeEach(func() {
				moverConfig := volsyncv1alpha1.MoverConfig{}
				utils.UpdatePodTemplateSpecFromMoverConfig(podTemplateSpec, moverConfig, defaultResourceRequirements)

				// Pod template spec should essentially be unchanged
				Expect(podTemplateSpec.Spec.SecurityContext).To(BeNil())
			})
			It("Should have empty resource requirements set", func() {
				// ResourceRequirements should be default (empty value)
				Expect(podTemplateSpec.Spec.Containers[0].Resources).To(Equal(corev1.ResourceRequirements{}))
			})

			When("Default resourceRequirements with limits are used", func() {
				BeforeEach(func() {
					defaultResourceRequirements = corev1.ResourceRequirements{
						Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("1Gi")},
					}
				})
				It("Should use the limits from the default requirements", func() {
					Expect(podTemplateSpec.Spec.Containers[0].Resources).To(Equal(corev1.ResourceRequirements{
						Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("1Gi")},
					}))
				})
			})
			When("podTemplateSpec has podLabels", func() {
				It("Should not update the podTemplateSpec and labels should be retained", func() {
					Expect(podTemplateSpec.Labels).To(Equal(existingLabelsOrig))
				})
			})
			When("podlabels are nil", func() {
				BeforeEach(func() {
					podTemplateSpec.Labels = nil
				})
				It("Should not fail, podLabels should be empty in the podtemplatespec", func() {
					Expect(podTemplateSpec.Labels).To(Equal(map[string]string{}))
				})
			})
		})

		When("moverConfig has a securityContext set", func() {
			var moverConfig volsyncv1alpha1.MoverConfig
			var customMoverSecurityContext *corev1.PodSecurityContext

			BeforeEach(func() {
				customMoverSecurityContext = &corev1.PodSecurityContext{
					RunAsUser: ptr.To[int64](20),
					FSGroup:   ptr.To[int64](40),
				}

				moverConfig = volsyncv1alpha1.MoverConfig{
					MoverSecurityContext: customMoverSecurityContext,
				}
			})

			It("Should update the securityContext in the podTemplateSpec", func() {
				utils.UpdatePodTemplateSpecFromMoverConfig(podTemplateSpec, moverConfig, corev1.ResourceRequirements{})
				Expect(podTemplateSpec.Spec.SecurityContext).To(Equal(customMoverSecurityContext))
			})
		})

		When("moverConfig has resourceRequirements and pod labels", func() {
			var moverConfig volsyncv1alpha1.MoverConfig
			var customLabels map[string]string
			var customResources *corev1.ResourceRequirements

			BeforeEach(func() {
				customLabels = map[string]string{
					"customCondimentLabel1": "pickles",
					"customCondimentLabel2": "mustard",
				}

				customResources = &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("5m"),
						corev1.ResourceMemory: resource.MustParse("32Mi"),
					},
				}

				moverConfig = volsyncv1alpha1.MoverConfig{
					MoverPodLabels: customLabels,
					MoverResources: customResources,
				}
			})

			It("Should update the podTemplateSpec", func() {
				utils.UpdatePodTemplateSpecFromMoverConfig(podTemplateSpec, moverConfig, corev1.ResourceRequirements{})

				for k, v := range existingLabelsOrig {
					Expect(podTemplateSpec.Labels[k]).To(Equal(v))
				}
				for k, v := range customLabels {
					Expect(podTemplateSpec.Labels[k]).To(Equal(v))
				}
				Expect(podTemplateSpec.Spec.Containers[0].Resources.Limits).To(Equal(customResources.Limits))
				Expect(podTemplateSpec.Spec.Containers[0].Resources.Requests).To(Equal(customResources.Requests))
			})

			When("the default resource requirements are set", func() {
				It("Should still use the user-supplied resourceRequirements", func() {
					defaultRequirements := corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("10m"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("20Mi"),
						},
					}
					utils.UpdatePodTemplateSpecFromMoverConfig(podTemplateSpec, moverConfig, defaultRequirements)
					Expect(podTemplateSpec.Spec.Containers[0].Resources.Limits).To(Equal(customResources.Limits))
					Expect(podTemplateSpec.Spec.Containers[0].Resources.Requests).To(Equal(customResources.Requests))
				})
			})
		})

		When("moverConfig has a podAffinity set", func() {
			var moverConfig volsyncv1alpha1.MoverConfig
			var customMoverAffinity *corev1.Affinity

			BeforeEach(func() {
				customMoverAffinity = &corev1.Affinity{
					NodeAffinity: &corev1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
							NodeSelectorTerms: []corev1.NodeSelectorTerm{
								{
									MatchFields: []corev1.NodeSelectorRequirement{
										{
											Key:      "a-key",
											Operator: corev1.NodeSelectorOpIn,
											Values:   []string{"a-label"},
										},
									},
								},
							},
						}},
					PodAntiAffinity: &corev1.PodAntiAffinity{
						PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
							{
								Weight: 1,
								PodAffinityTerm: corev1.PodAffinityTerm{
									LabelSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"app.kubernetes.io/created-by": "volsync",
										},
									},
									TopologyKey: "kubernetes.io/hostname",
								},
							},
						},
					},
				}

				moverConfig = volsyncv1alpha1.MoverConfig{
					MoverAffinity: customMoverAffinity,
				}
			})

			It("Should update the affinity in the podTemplateSpec", func() {
				utils.UpdatePodTemplateSpecFromMoverConfig(podTemplateSpec, moverConfig, corev1.ResourceRequirements{})
				Expect(podTemplateSpec.Spec.Affinity).To(Equal(customMoverAffinity))
			})
		})

		When("moverConfig has no podAffinity set", func() {
			var moverConfig volsyncv1alpha1.MoverConfig
			var podSpecAffinity corev1.Affinity

			BeforeEach(func() {
				moverConfig = volsyncv1alpha1.MoverConfig{
					MoverAffinity: nil,
				}

				podSpecAffinity = corev1.Affinity{
					NodeAffinity: &corev1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
							NodeSelectorTerms: []corev1.NodeSelectorTerm{
								{
									MatchFields: []corev1.NodeSelectorRequirement{
										{
											Key:      "a-key",
											Operator: corev1.NodeSelectorOpIn,
											Values:   []string{"a-label"},
										},
									},
								},
							},
						}},
				}
				podTemplateSpec.Spec.Affinity = &podSpecAffinity
			})

			It("Should not remove the affinity in the podTemplateSpec", func() {
				utils.UpdatePodTemplateSpecFromMoverConfig(podTemplateSpec, moverConfig, corev1.ResourceRequirements{})
				Expect(podTemplateSpec.Spec.Affinity).To(Equal(&podSpecAffinity))
			})
		})

	})

	Describe("ValidateMoverVolumes", func() {
		logger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))
		var ns *corev1.Namespace

		BeforeEach(func() {
			// Create namespace for test
			ns = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "namespc-",
				},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())
			Expect(ns.Name).NotTo(BeEmpty())
		})
		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
		})

		When("moverVolumes is empty", func() {
			It("ValidateMoverVolumes should succeed", func() {
				Expect(utils.ValidateMoverVolumes(
					ctx, k8sClient, logger, ns.GetName(), []volsyncv1alpha1.MoverVolume{})).To(Succeed())
			})
		})

		When("moverVolumes is nil", func() {
			It("ValidateMoverVolumes should succeed", func() {
				Expect(utils.ValidateMoverVolumes(
					ctx, k8sClient, logger, ns.GetName(), nil)).To(Succeed())
			})
		})

		When("moverVolumes has a volume source that contains PVCs and Secrets", func() {
			var moverVolumes []volsyncv1alpha1.MoverVolume

			BeforeEach(func() {
				moverVolumes = []volsyncv1alpha1.MoverVolume{
					{
						Name: "pvc1mnt",
						VolumeSource: volsyncv1alpha1.MoverVolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "pvc1",
							},
						},
					},
					{
						Name: "sec1mnt",
						VolumeSource: volsyncv1alpha1.MoverVolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: "sec1",
							},
						},
					},
				}
			})

			When("a PVC does not exist", func() {
				It("ValidateMoverVolumes should fail", func() {
					err := utils.ValidateMoverVolumes(
						ctx, k8sClient, logger, ns.GetName(), moverVolumes)

					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(ContainSubstring("not found")))
					Expect(err).To(MatchError(ContainSubstring("pvc1")))
				})
			})

			When("a secret does not exist", func() {
				BeforeEach(func() {
					// Create the PVC so validation proceeds to the secret
					pvc := &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "pvc1",
							Namespace: ns.Name,
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							AccessModes: []corev1.PersistentVolumeAccessMode{
								corev1.ReadWriteMany,
							},
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: resource.MustParse("1Gi"),
								},
							},
						},
					}
					Expect(k8sClient.Create(ctx, pvc)).To(Succeed())
				})

				When("a secret does not exist", func() {
					It("ValidateMoverVolumes should fail", func() {
						err := utils.ValidateMoverVolumes(
							ctx, k8sClient, logger, ns.GetName(), moverVolumes)

						Expect(err).To(HaveOccurred())
						Expect(err).To(MatchError(ContainSubstring("not found")))
						Expect(err).To(MatchError(ContainSubstring("sec1")))
					})
				})

				When("all mover volumes exist", func() {
					BeforeEach(func() {
						// Create the secret so all mover volumes exist
						sec := &corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "sec1",
								Namespace: ns.Name,
							},
							StringData: map[string]string{
								"key1": "value1",
							},
						}
						Expect(k8sClient.Create(ctx, sec)).To(Succeed())
					})

					It("ValidateMoverVolumes should succeed", func() {
						Expect(utils.ValidateMoverVolumes(
							ctx, k8sClient, logger, ns.GetName(), moverVolumes)).To(Succeed())
					})
				})
			})
		})
	})

	Describe("UpdatePodTemplateSpecWithMoverVolumes", func() {
		When("no pod template spec", func() {
			It("should not fail", func() {
				utils.UpdatePodTemplateSpecWithMoverVolumes(nil, []volsyncv1alpha1.MoverVolume{})
			})
		})

		var podTemplateSpec *corev1.PodTemplateSpec

		var origVolume corev1.Volume
		var origVolumeMount corev1.VolumeMount

		BeforeEach(func() {
			// Save the original volume and volume mount so we can compare later
			origVolume = corev1.Volume{
				Name: "test-volume-1",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{},
				},
			}
			origVolumeMount = corev1.VolumeMount{
				Name:      "test-volume-1",
				MountPath: "/data",
			}

			// Make a copy
			volume := origVolume.DeepCopy()
			volumeMount := origVolumeMount.DeepCopy()

			podTemplateSpec = &corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-podtemplatespec",
					Namespace: "test-ns",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						// 1 container for this test - all movers currently using
						// only 1 container
						{
							Name:    "testcontainer0",
							Image:   "quay.io/testtesttest/test:test",
							Command: []string{"takeovertheworld.sh", "now"},
							VolumeMounts: []corev1.VolumeMount{
								*volumeMount,
							},
						},
					},
					Volumes: []corev1.Volume{
						*volume,
					},
				},
			}
		})

		When("moverVolumes is empty", func() {
			It("should not fail", func() {
				utils.UpdatePodTemplateSpecWithMoverVolumes(podTemplateSpec, []volsyncv1alpha1.MoverVolume{})
				Expect(podTemplateSpec.Spec.Containers[0].VolumeMounts).To(Equal([]corev1.VolumeMount{origVolumeMount}))
				Expect(podTemplateSpec.Spec.Volumes).To(Equal([]corev1.Volume{origVolume}))
			})
		})

		When("moverVolumes is nil", func() {
			It("should not fail", func() {
				utils.UpdatePodTemplateSpecWithMoverVolumes(podTemplateSpec, nil)
				Expect(podTemplateSpec.Spec.Containers[0].VolumeMounts).To(Equal([]corev1.VolumeMount{origVolumeMount}))
				Expect(podTemplateSpec.Spec.Volumes).To(Equal([]corev1.Volume{origVolume}))
			})
		})

		// nolint:dupl
		When("moverVolumes has a volume source that is a PVC", func() {
			It("should mount the pvc in the pod template spec", func() {
				utils.UpdatePodTemplateSpecWithMoverVolumes(podTemplateSpec, []volsyncv1alpha1.MoverVolume{
					{
						Name: "test-volume-1",
						VolumeSource: volsyncv1alpha1.MoverVolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "addl-pvc-1",
							},
						},
					},
				})

				specVolumeMounts := podTemplateSpec.Spec.Containers[0].VolumeMounts
				specVolumes := podTemplateSpec.Spec.Volumes

				Expect(len(specVolumeMounts)).To(Equal(2))
				Expect(len(specVolumes)).To(Equal(2))

				Expect(specVolumeMounts[0]).To(Equal(origVolumeMount))
				Expect(specVolumes[0]).To(Equal(origVolume))

				// new PVC should be added
				Expect(specVolumeMounts[1]).To(Equal(corev1.VolumeMount{
					Name:      "u-test-volume-1",
					MountPath: "/mnt/test-volume-1",
				}))
				Expect(specVolumes[1]).To(Equal(corev1.Volume{
					Name: "u-test-volume-1",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "addl-pvc-1",
						},
					},
				}))
			})
		})

		// nolint:dupl // Same as above, but for a secret
		When("moverVolumes has a volume source that is a secret", func() {
			It("should mount them all in the pod template spec", func() {
				utils.UpdatePodTemplateSpecWithMoverVolumes(podTemplateSpec, []volsyncv1alpha1.MoverVolume{
					{
						Name: "test-sec-1",
						VolumeSource: volsyncv1alpha1.MoverVolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: "addl-sec1",
							},
						},
					},
				})

				specVolumeMounts := podTemplateSpec.Spec.Containers[0].VolumeMounts
				specVolumes := podTemplateSpec.Spec.Volumes

				Expect(len(specVolumeMounts)).To(Equal(2))
				Expect(len(specVolumes)).To(Equal(2))

				Expect(specVolumeMounts[0]).To(Equal(origVolumeMount))
				Expect(specVolumes[0]).To(Equal(origVolume))

				// new Secret should be added
				Expect(specVolumeMounts[1]).To(Equal(corev1.VolumeMount{
					Name:      "u-test-sec-1",
					MountPath: "/mnt/test-sec-1",
				}))
				Expect(specVolumes[1]).To(Equal(corev1.Volume{
					Name: "u-test-sec-1",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: "addl-sec1",
						},
					},
				}))
			})
		})

		When("moverVolumes has multiple volume sources ", func() {
			It("should mount the secret in the pod template spec", func() {
				utils.UpdatePodTemplateSpecWithMoverVolumes(podTemplateSpec, []volsyncv1alpha1.MoverVolume{
					{
						Name: "test-sec-1",
						VolumeSource: volsyncv1alpha1.MoverVolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: "addl-sec1",
							},
						},
					},
					{
						Name: "test-volume-1",
						VolumeSource: volsyncv1alpha1.MoverVolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "addl-pvc-1",
							},
						},
					},
					{
						// More complicated secret
						Name: "s2",
						VolumeSource: volsyncv1alpha1.MoverVolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: "my-secret-2",
								Items: []corev1.KeyToPath{
									{
										Key:  "key1",
										Path: "path1",
									},
									{
										Key:  "key2",
										Path: "path2",
									},
								},
							},
						},
					},
				})

				specVolumeMounts := podTemplateSpec.Spec.Containers[0].VolumeMounts
				specVolumes := podTemplateSpec.Spec.Volumes

				Expect(len(specVolumeMounts)).To(Equal(4))
				Expect(len(specVolumes)).To(Equal(4))

				Expect(specVolumeMounts[0]).To(Equal(origVolumeMount))
				Expect(specVolumes[0]).To(Equal(origVolume))

				// new Secret should be added
				Expect(specVolumeMounts[1]).To(Equal(corev1.VolumeMount{
					Name:      "u-test-sec-1",
					MountPath: "/mnt/test-sec-1",
				}))
				Expect(specVolumes[1]).To(Equal(corev1.Volume{
					Name: "u-test-sec-1",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: "addl-sec1",
						},
					},
				}))

				// new PVC should be added
				Expect(specVolumeMounts[2]).To(Equal(corev1.VolumeMount{
					Name:      "u-test-volume-1",
					MountPath: "/mnt/test-volume-1",
				}))
				Expect(specVolumes[2]).To(Equal(corev1.Volume{
					Name: "u-test-volume-1",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "addl-pvc-1",
						},
					},
				}))

				// new secret should be added
				Expect(specVolumeMounts[3]).To(Equal(corev1.VolumeMount{
					Name:      "u-s2",
					MountPath: "/mnt/s2",
				}))
				Expect(specVolumes[3]).To(Equal(corev1.Volume{
					Name: "u-s2",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: "my-secret-2",
							Items: []corev1.KeyToPath{
								{
									Key:  "key1",
									Path: "path1",
								},
								{
									Key:  "key2",
									Path: "path2",
								},
							},
						},
					},
				}))
			})
		})
	})

	Describe("Name length limit tests", func() {
		var ns *corev1.Namespace
		var rd *volsyncv1alpha1.ReplicationDestination
		var rdlongname *volsyncv1alpha1.ReplicationDestination

		BeforeEach(func() {
			ns = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "utils-test-",
				},
			}

			Expect(k8sClient.Create(ctx, ns)).To(Succeed())
			Expect(ns.Name).NotTo(BeEmpty())
		})

		JustBeforeEach(func() {
			// Create namespace for test
			rd = &volsyncv1alpha1.ReplicationDestination{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-rd-owner-obj-",
					Namespace:    ns.Name,
				},
				Spec: volsyncv1alpha1.ReplicationDestinationSpec{},
			}

			rdlongname = &volsyncv1alpha1.ReplicationDestination{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-rd-owner-obj-aaa-bbb-ccc-this-name-is-greater-than-63chars-",
					Namespace:    ns.Name,
				},
				Spec: volsyncv1alpha1.ReplicationDestinationSpec{},
			}

			Expect(k8sClient.Create(ctx, rd)).To(Succeed())
			Expect(k8sClient.Create(ctx, rdlongname)).To(Succeed())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
		})

		Describe("Job name test", func() {
			It("Should use the prefix + owner CR name when <= 63 chars", func() {
				jobName := utils.GetJobName("myprefix-", rd)
				Expect(jobName).To(Equal("myprefix-" + rd.GetName()))
			})

			It("Should use prefix + hashed owner name when > 63 chars", func() {
				jobName := utils.GetJobName("myprefix-", rdlongname)
				hashedName := utils.GetHashedName(rdlongname.GetName())
				Expect(len(hashedName)).To(Equal(8))
				Expect(jobName).To(Equal("myprefix-" + hashedName))
			})
		})

		Describe("Service name test", func() {
			It("Should use the prefix + owner CR name when <= 63 chars", func() {
				jobName := utils.GetServiceName("myprefix-", rd)
				Expect(jobName).To(Equal("myprefix-" + rd.GetName()))
			})

			It("Should use prefix + hashed owner name when > 63 chars", func() {
				jobName := utils.GetServiceName("myprefix-", rdlongname)
				Expect(jobName).To(Equal("myprefix-" + utils.GetHashedName(rdlongname.GetName())))
			})
		})

		Describe("Label value test", func() {
			It("Should use the prefix + owner CR name when <= 63 chars", func() {
				jobName := utils.GetOwnerNameLabelValue("myprefix-", rd)
				Expect(jobName).To(Equal("myprefix-" + rd.GetName()))
			})

			It("Should use prefix + hashed owner name when > 63 chars", func() {
				jobName := utils.GetOwnerNameLabelValue("myprefix-", rdlongname)
				Expect(jobName).To(Equal("myprefix-" + utils.GetHashedName(rdlongname.GetName())))
			})
		})
	})
})
