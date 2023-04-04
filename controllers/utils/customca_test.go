/*
Copyright 2023 The VolSync authors.

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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/controllers/utils"
)

var _ = Describe("customca tests", func() {
	logger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))
	var testNamespace *corev1.Namespace

	BeforeEach(func() {
		// Create namespace for test
		testNamespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "ca-test-ns-",
			},
		}
		Expect(k8sClient.Create(ctx, testNamespace)).To(Succeed())
	})
	AfterEach(func() {
		// All resources are namespaced, so this should clean it all up
		Expect(k8sClient.Delete(ctx, testNamespace)).To(Succeed())
	})

	var customCASpec volsyncv1alpha1.CustomCASpec
	var customCAObj utils.CustomCAObject
	var errValidateCustomCA error

	JustBeforeEach(func() {
		// ValidateCustomCA - will create a customCAObj if successful
		customCAObj, errValidateCustomCA = utils.ValidateCustomCA(ctx, k8sClient, logger, testNamespace.GetName(),
			customCASpec)
	})

	Context("When default (empty) customCASpec is used", func() {
		BeforeEach(func() {
			customCASpec = volsyncv1alpha1.CustomCASpec{}
		})

		It("ValidateCustomCA should return an empty customCAObj", func() {
			Expect(errValidateCustomCA).NotTo(HaveOccurred()) // No error should happen
			Expect(customCAObj).To(BeNil())
		})
	})

	Context("When a CustomCASpec with a secret is used", func() {
		customCaSecName := "test-customca-sec"
		customCaKeyName := "myca.crt"

		BeforeEach(func() {
			customCASpec = volsyncv1alpha1.CustomCASpec{
				SecretName: customCaSecName,
				Key:        customCaKeyName,
			}
		})

		Context("When the secret does not exist", func() {
			It("Should fail validation", func() {
				Expect(errValidateCustomCA).To(HaveOccurred())
				Expect(kerrors.IsNotFound(errValidateCustomCA)).To(BeTrue())
				Expect(customCAObj).To(BeNil())
			})
		})

		Context("When the secret is missing the key", func() {
			BeforeEach(func() {
				// Create the secret
				customCaSec := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      customCaSecName,
						Namespace: testNamespace.GetName(),
					},
					StringData: map[string]string{
						"notthekey": "notcustomca",
					},
				}
				Expect(k8sClient.Create(ctx, customCaSec)).To(Succeed())
			})

			It("Should fail validation", func() {
				Expect(errValidateCustomCA).To(HaveOccurred())
				Expect(errValidateCustomCA.Error()).To(ContainSubstring("secret is missing field: " + customCaKeyName))

				Expect(customCAObj).To(BeNil())
			})
		})

		Context("When the secret exists with the key", func() {
			BeforeEach(func() {
				// Create the secret
				customCaSec := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      customCaSecName,
						Namespace: testNamespace.GetName(),
					},
					StringData: map[string]string{
						customCaKeyName: "mycustomcastringcontents",
					},
				}
				Expect(k8sClient.Create(ctx, customCaSec)).To(Succeed())
			})

			It("Should pass validation and GetVolumeSource should return the proper spec", func() {
				Expect(errValidateCustomCA).NotTo(HaveOccurred())
				Expect(customCAObj).NotTo(BeNil())

				volSource := customCAObj.GetVolumeSource("crtfile.crt")

				Expect(volSource).To(Equal(corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: customCaSecName,
						Items: []corev1.KeyToPath{
							{Key: customCaKeyName, Path: "crtfile.crt"},
						},
					},
				}))
			})
		})
	})

	Context("When a CustomCASpec with a configmap is used", func() {
		customCaCMName := "test-customca-cm"
		customCaKeyName := "myca.crt"

		BeforeEach(func() {
			customCASpec = volsyncv1alpha1.CustomCASpec{
				ConfigMapName: customCaCMName,
				Key:           customCaKeyName,
			}
		})

		Context("When the configmap does not exist", func() {
			It("Should fail validation", func() {
				Expect(errValidateCustomCA).To(HaveOccurred())
				Expect(kerrors.IsNotFound(errValidateCustomCA)).To(BeTrue())
				Expect(customCAObj).To(BeNil())
			})
		})

		Context("When the configmap is missing the key", func() {
			BeforeEach(func() {
				// Create the configmap
				customCaCM := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      customCaCMName,
						Namespace: testNamespace.GetName(),
					},
					Data: map[string]string{
						"notthekey": "notcustomca",
					},
				}
				Expect(k8sClient.Create(ctx, customCaCM)).To(Succeed())
			})

			It("Should fail validation", func() {
				Expect(errValidateCustomCA).To(HaveOccurred())
				Expect(errValidateCustomCA.Error()).To(ContainSubstring("configmap is missing field: " + customCaKeyName))

				Expect(customCAObj).To(BeNil())
			})
		})

		Context("When the configmap exists with the key", func() {
			BeforeEach(func() {
				// Create the configmap
				customCaCM := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      customCaCMName,
						Namespace: testNamespace.GetName(),
					},
					Data: map[string]string{
						customCaKeyName: "mycustomcastringcontents",
					},
				}
				Expect(k8sClient.Create(ctx, customCaCM)).To(Succeed())
			})

			It("Should pass validation and GetVolumeSource should return the proper spec", func() {
				Expect(errValidateCustomCA).NotTo(HaveOccurred())
				Expect(customCAObj).NotTo(BeNil())

				volSource := customCAObj.GetVolumeSource("crtfile.crt")

				Expect(volSource).To(Equal(corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: customCaCMName},
						Items: []corev1.KeyToPath{
							{Key: customCaKeyName, Path: "crtfile.crt"},
						},
					},
				}))
			})
		})
	})

})
