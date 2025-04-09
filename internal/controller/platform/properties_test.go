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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ocpsecurityv1 "github.com/openshift/api/security/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/backube/volsync/internal/controller/utils"
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
		bytes, err := os.ReadFile("../test/scc-crd.yml")
		// Make sure we successfully read the file
		Expect(err).NotTo(HaveOccurred())
		Expect(len(bytes)).To(BeNumerically(">", 0))
		sccCRD = &apiextensionsv1.CustomResourceDefinition{}
		err = yaml.Unmarshal(bytes, sccCRD)
		Expect(err).NotTo(HaveOccurred())
		// Parsed yaml correctly
		Expect(sccCRD.Name).To(Equal("securitycontextconstraints.security.openshift.io"))
		Expect(k8sClient.Create(ctx, sccCRD)).NotTo(HaveOccurred())
		Eventually(func() bool {
			// Getting sccs list should return empty list (no error)
			sccList := ocpsecurityv1.SecurityContextConstraintsList{}
			err := k8sClient.List(ctx, &sccList)
			if err != nil {
				return false
			}
			return len(sccList.Items) == 0
		}, 5*time.Second).Should(BeTrue())
		priv = &ocpsecurityv1.SecurityContextConstraints{
			ObjectMeta: metav1.ObjectMeta{
				Name: "privileged",
			},
		}
		Expect(k8sClient.Create(ctx, priv)).To(Succeed())
	})
	AfterEach(func() {
		deleteScc(priv)

		Expect(k8sClient.Delete(ctx, sccCRD)).To(Succeed())
		Eventually(func() bool {
			// CRD can take a while to cleanup and leak into subsequent tests that run in the same process, wait
			// to ensure it's gone
			reloadErr := k8sClient.Get(ctx, client.ObjectKeyFromObject(sccCRD), sccCRD)
			if !kerrors.IsNotFound(reloadErr) {
				return false // SCC CRD is still there, keep trying
			}

			// Doublecheck sccs gone
			sccList := ocpsecurityv1.SecurityContextConstraintsList{}
			getSccErr := k8sClient.List(ctx, &sccList)
			return kerrors.IsNotFound(getSccErr)
		}, 60*time.Second, 250*time.Millisecond).Should(BeTrue())
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
			deleteScc(rv2)
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

			AfterEach(func() {
				// Make sure we clean up the scc if it was successfully created
				createdScc := &ocpsecurityv1.SecurityContextConstraints{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: sccName}, createdScc)
				if err != nil {
					deleteScc(createdScc)
				}
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

			AfterEach(func() {
				// clean up the scc after test
				existingScc := &ocpsecurityv1.SecurityContextConstraints{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: existingSccName}, existingScc)).To(Succeed())
				deleteScc(existingScc)
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

const maxWait = 10 * time.Second
const interval = 250 * time.Millisecond

func deleteScc(scc *ocpsecurityv1.SecurityContextConstraints) {
	// Delete's scc and uses Eventually() to check that it's completed the deletion
	// Doing this to avoid timing issues with multiple tests that may share the same control plane
	Expect(k8sClient.Delete(ctx, scc)).To(Succeed())

	Eventually(func() bool {
		return kerrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(scc), scc))
	}, maxWait, interval).Should(BeTrue())
}
