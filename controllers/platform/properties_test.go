/*
Copyright 2021 The VolSync authors.

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

package platform

import (
	"os"
	"time"

	"github.com/backube/volsync/controllers/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	//"gopkg.in/yaml.v2"
	ocpsecurityv1 "github.com/openshift/api/security/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var logger = zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))

var _ = Describe("A cluster w/o StorageContextConstraints", func() {
	It("should NOT be detected as OpenShift", func() {
		props, err := GetProperties(ctx, k8sClient, logger)
		Expect(err).NotTo(HaveOccurred())
		Expect(props.IsOpenShift).To(BeFalse())
		Expect(props.HasSCCRestrictedV2).To(BeFalse())
	})

	It("EnsureVolSyncMoverSCCIfOpenShift should not fail", func() {
		// In main.go this will be embedded - but for tests, read in the file manually
		sccRaw, err := os.ReadFile("../../config/openshift/mover_scc.yaml")
		Expect(err).NotTo(HaveOccurred())

		Expect(EnsureVolSyncMoverSCCIfOpenShift(ctx, k8sClient, logger,
			"some-scc-name", sccRaw)).To(Succeed())
	})
})

var _ = Describe("A cluster w/ StorageContextConstraints", func() {
	var sccCRD *apiextensionsv1.CustomResourceDefinition
	var priv *ocpsecurityv1.SecurityContextConstraints
	BeforeEach(func() {
		// https://github.com/openshift/api/blob/master/security/v1/0000_03_security-openshift_01_scc.crd.yaml
		bytes, err := os.ReadFile("scc-crd.yml")
		// Make sure we successfully read the file
		Expect(err).NotTo(HaveOccurred())
		Expect(len(bytes)).To(BeNumerically(">", 0))
		sccCRD = &apiextensionsv1.CustomResourceDefinition{}
		err = yaml.Unmarshal(bytes, sccCRD)
		Expect(err).NotTo(HaveOccurred())
		// Parsed yaml correctly
		Expect(sccCRD.Name).To(Equal("securitycontextconstraints.security.openshift.io"))
		err = k8sClient.Create(ctx, sccCRD)
		Expect(kerrors.IsAlreadyExists(err) || err == nil).To(BeTrue())
		Eventually(func() bool {
			// Getting a random SCC should return NotFound as opposed to NoKindMatchError
			scc := ocpsecurityv1.SecurityContextConstraints{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: "zzzz"}, &scc)
			return kerrors.IsNotFound(err)
		}, 5*time.Second).Should(BeTrue())
		priv = &ocpsecurityv1.SecurityContextConstraints{
			ObjectMeta: metav1.ObjectMeta{
				Name: "privileged",
			},
		}
		Expect(k8sClient.Create(ctx, priv)).To(Succeed())
	})
	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, priv)).To(Succeed())
	})
	It("should be detected as OpenShift", func() {
		props, err := GetProperties(ctx, k8sClient, logger)
		Expect(err).NotTo(HaveOccurred())
		Expect(props.IsOpenShift).To(BeTrue())
	})
	It("should not detect restricted-v2", func() {
		props, err := GetProperties(ctx, k8sClient, logger)
		Expect(err).NotTo(HaveOccurred())
		Expect(props.HasSCCRestrictedV2).To(BeFalse())
	})
	When("restricted-v2 exists", func() {
		var rv2 *ocpsecurityv1.SecurityContextConstraints
		BeforeEach(func() {
			rv2 = &ocpsecurityv1.SecurityContextConstraints{
				ObjectMeta: metav1.ObjectMeta{
					Name: "restricted-v2",
				},
			}
			Expect(k8sClient.Create(ctx, rv2)).To(Succeed())
		})
		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, rv2)).To(Succeed())
		})
		It("should be detected", func() {
			props, err := GetProperties(ctx, k8sClient, logger)
			Expect(err).NotTo(HaveOccurred())
			Expect(props.HasSCCRestrictedV2).To(BeTrue())
		})
	})

	Describe("EnsureVolSyncMoverSCCIfOpenShift tests", func() {
		var sccRaw []byte
		BeforeEach(func() {
			// In main.go this will be embedded - but for tests, read in the file manually
			var err error
			sccRaw, err = os.ReadFile("../../config/openshift/mover_scc.yaml")
			Expect(err).NotTo(HaveOccurred())
		})

		When("The scc does not exist", func() {
			sccName := "test-scc-1"
			BeforeEach(func() {
				// doublecheck it doesn't exist
				existingScc := &ocpsecurityv1.SecurityContextConstraints{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: sccName}, existingScc)
				Expect(kerrors.IsNotFound(err)).To(BeTrue())
			})
			It("Should be created and have the volsync owned-by label", func() {
				Expect(EnsureVolSyncMoverSCCIfOpenShift(ctx, k8sClient, logger,
					sccName, sccRaw)).To(Succeed())

				newScc := &ocpsecurityv1.SecurityContextConstraints{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sccName}, newScc)).To(Succeed())

				// Make sure the owned by volsync label is there
				Expect(utils.IsOwnedByVolsync(newScc)).To(BeTrue())

				// Check some properties (note these checks may need to change if the mover in
				// config/openshift/mover_scc.yaml is updated)
				Expect(newScc.AllowHostDirVolumePlugin).To(BeFalse())
				Expect(newScc.FSGroup.Type).To(Equal(ocpsecurityv1.FSGroupStrategyRunAsAny))
				Expect(newScc.SELinuxContext.Type).To(Equal(ocpsecurityv1.SELinuxStrategyMustRunAs))
			})
		})

		When("The scc already exists", func() {
			existingSccName := "test-scc-2"
			BeforeEach(func() {
				// Let the func create the SCC for us
				Expect(EnsureVolSyncMoverSCCIfOpenShift(ctx, k8sClient, logger,
					existingSccName, sccRaw)).To(Succeed())

				existingScc := &ocpsecurityv1.SecurityContextConstraints{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: existingSccName}, existingScc)).To(Succeed())

				// Now modify the SCC so it doesn't match our mover_scc.yaml
				existingScc.AllowedCapabilities = []corev1.Capability{
					"AUDIT_WRITE",
					"FAKE_CAPABILITY",
				}

				existingScc.AllowHostDirVolumePlugin = true

				Expect(k8sClient.Update(ctx, existingScc)).To(Succeed())
			})

			When("The scc has the volsync owned by label", func() {
				BeforeEach(func() {
					existingScc := &ocpsecurityv1.SecurityContextConstraints{}
					Expect(k8sClient.Get(ctx, types.NamespacedName{Name: existingSccName}, existingScc)).To(Succeed())
					Expect(utils.IsOwnedByVolsync(existingScc)).To(BeTrue())
				})
				It("Should update the existing scc", func() {
					Expect(EnsureVolSyncMoverSCCIfOpenShift(ctx, k8sClient, logger,
						existingSccName, sccRaw)).To(Succeed())

					updatedScc := &ocpsecurityv1.SecurityContextConstraints{}
					Expect(k8sClient.Get(ctx, types.NamespacedName{Name: existingSccName}, updatedScc)).To(Succeed())
					Expect(utils.IsOwnedByVolsync(updatedScc)).To(BeTrue()) // Label should still be there

					Expect(updatedScc.AllowHostDirVolumePlugin).To(BeFalse()) // Was changed to true in beforeeach
					// Check arrays to make sure they are set correctly
					Expect(updatedScc.AllowedCapabilities).Should(ContainElement(corev1.Capability("AUDIT_WRITE")))
					Expect(updatedScc.AllowedCapabilities).Should(ContainElement(corev1.Capability("CHOWN")))

					// Old capability not in our mover_scc.yaml should be removed
					Expect(updatedScc.AllowedCapabilities).ShouldNot(ContainElement(corev1.Capability("FAKE_CAPABILITY")))
				})
			})

			When("The scc does not have the volsync owned by label", func() {
				BeforeEach(func() {
					existingScc := &ocpsecurityv1.SecurityContextConstraints{}
					Expect(k8sClient.Get(ctx, types.NamespacedName{Name: existingSccName}, existingScc)).To(Succeed())

					// Remove volsync owned by label
					utils.RemoveOwnedByVolSync(existingScc)
					Expect(k8sClient.Update(ctx, existingScc)).To(Succeed())
				})
				It("Should not update or modify the existing scc", func() {
					// re-load just before
					existingScc := &ocpsecurityv1.SecurityContextConstraints{}
					Expect(k8sClient.Get(ctx, types.NamespacedName{Name: existingSccName}, existingScc)).To(Succeed())

					Expect(EnsureVolSyncMoverSCCIfOpenShift(ctx, k8sClient, logger,
						existingSccName, sccRaw)).To(Succeed())

					reloadedScc := &ocpsecurityv1.SecurityContextConstraints{}
					Expect(k8sClient.Get(ctx, types.NamespacedName{Name: existingSccName}, reloadedScc)).To(Succeed())
					Expect(utils.IsOwnedByVolsync(reloadedScc)).To(BeFalse()) // Label should still *not* be there

					Expect(existingScc.Generation).To(Equal(reloadedScc.Generation))

					Expect(reloadedScc.AllowHostDirVolumePlugin).To(BeTrue())
					// Check arrays to make sure they were not modified
					Expect(len(reloadedScc.AllowedCapabilities)).To(Equal(2))
					Expect(reloadedScc.AllowedCapabilities).Should(ContainElement(corev1.Capability("AUDIT_WRITE")))
					Expect(reloadedScc.AllowedCapabilities).Should(ContainElement(corev1.Capability("FAKE_CAPABILITY")))
				})
			})
		})
	})
})
