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
	"context"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	ocpconfigv1 "github.com/openshift/api/config/v1"
	ocpsecurityv1 "github.com/openshift/api/security/v1"
	ocptls "github.com/openshift/controller-runtime-common/pkg/tls"

	"github.com/backube/volsync/internal/controller/utils"
)

var logger = zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))

var _ = Describe("A cluster w/o StorageContextConstraints", func() {
	BeforeEach(func() {
		// Make sure we're not caching properties
		clearProperties()
	})
	AfterEach(func() {
		// Make sure we're not caching properties
		clearProperties()
	})

	It("should NOT be detected as OpenShift", func() {
		props, err := GetProperties(ctx, k8sClient, logger)
		Expect(err).NotTo(HaveOccurred())
		Expect(props.IsOpenShift).To(BeFalse())
	})

	It("EnsureVolSyncMoverSCCIfOpenShift should not fail", func() {
		// In main.go this will be embedded - but for tests, read in the file manually
		sccRaw, err := os.ReadFile("../../../cmd/openshift/mover_scc.yaml")
		Expect(err).NotTo(HaveOccurred())

		Expect(EnsureVolSyncMoverSCCIfOpenShift(ctx, k8sClient, logger,
			"some-scc-name", sccRaw)).To(Succeed())
	})

	It("GetTLSProfileIfOpenShift should not fail", func() {
		tlsProfileSpec, err := GetTLSProfileIfOpenShift(ctx, k8sClient, logger)
		Expect(err).NotTo(HaveOccurred())
		Expect(tlsProfileSpec).To(BeNil())
	})
})

var _ = Describe("A cluster w/ StorageContextConstraints", func() {
	BeforeEach(func() {
		// Make sure we're not caching properties
		clearProperties()
	})
	AfterEach(func() {
		// Make sure we're not caching properties
		clearProperties()
	})

	var sccCRD *apiextensionsv1.CustomResourceDefinition
	var ocpAPISvrCRD *apiextensionsv1.CustomResourceDefinition
	var priv *ocpsecurityv1.SecurityContextConstraints
	var clusterAPIServer *ocpconfigv1.APIServer
	BeforeEach(func() {
		// Install scc CRD - so we can query the API - this will let our tests think the cluster is OpenShift
		// https://github.com/openshift/api/blob/master/security/v1/0000_03_security-openshift_01_scc.crd.yaml
		bytes, err := os.ReadFile("../test/scc-crd.yml")
		// Make sure we successfully read the file
		Expect(err).NotTo(HaveOccurred())
		Expect(bytes).ToNot(BeEmpty())
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

		// Install openshift/config apiservers CRD - used to query for TLS Profiles
		// nolint:lll
		// https://github.com/openshift/api/blob/master/config/v1/zz_generated.crd-manifests/0000_10_config-operator_01_apiservers-Default.crd.yaml
		bytes, err = os.ReadFile("../test/openshift-apiservers-crd.yml")
		// Make sure we successfully read the file
		Expect(err).NotTo(HaveOccurred())
		Expect(bytes).ToNot(BeEmpty())
		ocpAPISvrCRD = &apiextensionsv1.CustomResourceDefinition{}
		err = yaml.Unmarshal(bytes, ocpAPISvrCRD)
		Expect(err).NotTo(HaveOccurred())
		// Parsed yaml correctly
		Expect(ocpAPISvrCRD.Name).To(Equal("apiservers.config.openshift.io"))
		Expect(k8sClient.Create(ctx, ocpAPISvrCRD)).To(Succeed())
		Eventually(func() bool {
			// Getting apiServers list should return empty list (no error)
			apiServerList := ocpconfigv1.APIServerList{}
			err := k8sClient.List(ctx, &apiServerList)
			if err != nil {
				return false
			}
			return len(apiServerList.Items) == 0
		}, 5*time.Second).Should(BeTrue())

		// Create API server named "cluster" - this is expected to always exist in OpenShift
		clusterAPIServer = &ocpconfigv1.APIServer{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cluster",
			},
		}
		Expect(k8sClient.Create(ctx, clusterAPIServer)).To(Succeed())
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

		deleteAPIServer(clusterAPIServer)

		Expect(k8sClient.Delete(ctx, ocpAPISvrCRD)).To(Succeed())
		Eventually(func() bool {
			// CRD can take a while to cleanup and leak into subsequent tests that run in the same process, wait
			// to ensure it's gone
			reloadErr := k8sClient.Get(ctx, client.ObjectKeyFromObject(ocpAPISvrCRD), ocpAPISvrCRD)
			if !kerrors.IsNotFound(reloadErr) {
				return false // APIServer CRD is still there, keep trying
			}

			// Doublecheck sccs gone
			apiSvrList := ocpsecurityv1.SecurityContextConstraintsList{}
			getAPISvrErr := k8sClient.List(ctx, &apiSvrList)
			return kerrors.IsNotFound(getAPISvrErr)
		}, 60*time.Second, 250*time.Millisecond).Should(BeTrue())

	})
	It("should be detected as OpenShift", func() {
		props, err := GetProperties(ctx, k8sClient, logger)
		Expect(err).NotTo(HaveOccurred())
		Expect(props.IsOpenShift).To(BeTrue())
	})

	Describe("EnsureVolSyncMoverSCCIfOpenShift tests", func() {
		var sccRaw []byte
		BeforeEach(func() {
			// In main.go this will be embedded - but for tests, read in the file manually
			var err error
			sccRaw, err = os.ReadFile("../../../cmd/openshift/mover_scc.yaml")
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
					Expect(reloadedScc.AllowedCapabilities).To(HaveLen(2))
					Expect(reloadedScc.AllowedCapabilities).Should(ContainElement(corev1.Capability("AUDIT_WRITE")))
					Expect(reloadedScc.AllowedCapabilities).Should(ContainElement(corev1.Capability("FAKE_CAPABILITY")))
				})
			})
		})
	})

	Describe("OpenShift TLS Profile tests", func() {
		When("No tls profile is set in the OpenShift APIServer", func() {
			It("Should get the default profile back", func() {
				tlsProfileSpec, err := GetTLSProfileIfOpenShift(ctx, k8sClient, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(tlsProfileSpec).NotTo(BeNil())
				// By default it should be set to the intermediate type
				Expect(tlsProfileSpec.MinTLSVersion).To(Equal(ocptls.DefaultMinTLSVersion))
				Expect(tlsProfileSpec.Ciphers).To(Equal(ocptls.DefaultTLSCiphers))
			})
		})

		When("A tls profile is set in the OpenShift APIServer", func() {
			BeforeEach(func() {
				clusterAPIServer.Spec.TLSSecurityProfile = &ocpconfigv1.TLSSecurityProfile{
					Modern: &ocpconfigv1.ModernTLSProfile{},
					Type:   ocpconfigv1.TLSProfileModernType,
				}
				Expect(k8sClient.Update(ctx, clusterAPIServer)).To(Succeed())
			})

			It("Should be returned", func() {
				tlsProfileSpec, err := GetTLSProfileIfOpenShift(ctx, k8sClient, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(tlsProfileSpec).NotTo(BeNil())
				// By default it should be set to the intermediate type
				Expect(tlsProfileSpec.MinTLSVersion).To(Equal(
					ocpconfigv1.TLSProfiles[ocpconfigv1.TLSProfileModernType].MinTLSVersion))
				Expect(tlsProfileSpec.Ciphers).To(Equal(
					ocpconfigv1.TLSProfiles[ocpconfigv1.TLSProfileModernType].Ciphers))
			})
		})

		When("a TLS profile watcher is running", func() {
			cancelFuncCalled := false

			var testManagerCancel context.CancelFunc

			BeforeEach(func() {
				testManager, err := ctrl.NewManager(cfg, ctrl.Options{
					Scheme:  scheme.Scheme,
					Metrics: metricsserver.Options{BindAddress: "0"},
				})
				Expect(err).ToNot(HaveOccurred())

				initialTLSProfileSpec, err := GetTLSProfileIfOpenShift(ctx, k8sClient, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(initialTLSProfileSpec).NotTo(BeNil())

				// Init tls security profile watcher and have it call "testCancelFunc()"
				// if the TLS profile has changed
				err = InitTLSSecurityProfileWatcherWithManager(testManager,
					*initialTLSProfileSpec, logger, func() {
						logger.Info("Test CancelFunc called")
						cancelFuncCalled = true
					})
				Expect(err).NotTo(HaveOccurred())

				var testManagerCtx context.Context
				testManagerCtx, testManagerCancel = context.WithCancel(ctx)
				// Start the manager
				go func() {
					defer GinkgoRecover()
					err = testManager.Start(testManagerCtx)
					Expect(err).NotTo(HaveOccurred())
				}()
			})
			AfterEach(func() {
				testManagerCancel()
			})

			It("Should not call cancel function if no TLS profile change", func() {
				clearProperties()
				tlsProfileSpec, err := GetTLSProfileIfOpenShift(ctx, k8sClient, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(tlsProfileSpec).NotTo(BeNil())
				Expect(cancelFuncCalled).To(BeFalse())
			})

			It("Should call the cancel function when the TLS profile changes", func() {
				clusterAPIServer.Spec.TLSSecurityProfile = &ocpconfigv1.TLSSecurityProfile{
					Modern: &ocpconfigv1.ModernTLSProfile{},
					Type:   ocpconfigv1.TLSProfileModernType,
				}
				Expect(k8sClient.Update(ctx, clusterAPIServer)).To(Succeed())

				Eventually(func() bool {
					return cancelFuncCalled
				}, maxWait, interval).Should(BeTrue())
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

func deleteAPIServer(apiSvr *ocpconfigv1.APIServer) {
	// Deletes apiserver CR and uses Eventually() to check that it's completed the deletion
	// Doing this to avoid timing issues with multiple tests that may share the same control plane
	Expect(k8sClient.Delete(ctx, apiSvr)).To(Succeed())

	Eventually(func() bool {
		return kerrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(apiSvr), apiSvr))
	}, maxWait, interval).Should(BeTrue())
}
