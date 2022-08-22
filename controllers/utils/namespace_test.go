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
	"fmt"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/controllers/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var _ = Describe("Namespace mover privilege tests", func() {
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

	When("The namespace has no privileged-movers annotation or a non true value", func() {
		// All these should be interpreted as false
		emptyValue := ""
		falseValue := "false"
		invalidValue := "somethingelse"

		// Values to use during the tests
		annotationValues := []*string{
			nil, // Don't set the annotation at all
			&emptyValue,
			&falseValue,
			&invalidValue,
		}

		for i := range annotationValues {
			annotationVal := annotationValues[i]
			valueTxt := "not set at all"
			if annotationVal != nil {
				valueTxt = "'" + *annotationVal + "'"
			}
			When(fmt.Sprintf("privileged-movers ns annotation is %s", valueTxt), func() {
				BeforeEach(func() {
					if annotationVal != nil {
						// Set the annotation to the desired value before running the test
						ns.Annotations = map[string]string{
							volsyncv1alpha1.PrivilegedMoversNamespaceAnnotation: *annotationVal,
						}

						Expect(k8sClient.Update(ctx, ns)).To(Succeed())
					}
				})

				It("Should indicate privileged movers are not allowed", func() {
					privilegedMoversOk, err := utils.PrivilegedMoversOk(ctx, k8sClient, logger, ns.GetName())
					Expect(err).NotTo(HaveOccurred())
					Expect(privilegedMoversOk).To(BeFalse())
				})
			})
		}
	})

	When("The namespace has the privileged-movers annotation set to 'true'", func() {
		BeforeEach(func() {
			// Set the annotation to the desired value before running the test
			ns.Annotations = map[string]string{
				volsyncv1alpha1.PrivilegedMoversNamespaceAnnotation: "true",
			}

			Expect(k8sClient.Update(ctx, ns)).To(Succeed())
		})

		It("Should indicate privileged movers are allowed", func() {
			privilegedMoversOk, err := utils.PrivilegedMoversOk(ctx, k8sClient, logger, ns.GetName())
			Expect(err).NotTo(HaveOccurred())
			Expect(privilegedMoversOk).To(BeTrue())
		})
	})
})
