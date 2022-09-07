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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	//"gopkg.in/yaml.v2"
	ocpsecurityv1 "github.com/openshift/api/security/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
})
