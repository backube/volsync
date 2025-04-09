/*
Copyright 2025 The VolSync authors.

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
	"github.com/onsi/gomega/gstruct"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/internal/controller/utils"
)

var _ = Describe("sahandler tests", func() {
	logger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))

	var namespace *corev1.Namespace
	var ownerRS *volsyncv1alpha1.ReplicationSource

	BeforeEach(func() {
		// Each test is run in its own namespace
		namespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "volsync-sahandler-test-",
			},
		}
		Expect(k8sClient.Create(ctx, namespace)).To(Succeed())
		Expect(namespace.Name).NotTo(BeEmpty())

		ownerRS = &volsyncv1alpha1.ReplicationSource{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "rs-test-",
				Namespace:    namespace.GetName(),
			},
			Spec: volsyncv1alpha1.ReplicationSourceSpec{},
		}
		Expect(k8sClient.Create(ctx, ownerRS)).To(Succeed())

	})

	AfterEach(func() {
		// All resources are namespaced, so this should clean it all up
		Expect(k8sClient.Delete(ctx, namespace)).To(Succeed())
	})

	var saHandler utils.SAHandler
	var isPrivileged bool
	var userSuppliedSA *string

	JustBeforeEach(func() {
		// Initialize the saHAndler
		saHandler = utils.NewSAHandler(k8sClient, ownerRS, true, isPrivileged, userSuppliedSA)
	})

	Describe("VolSync SAHandler tests", func() {
		var testVolsyncSystemNamespace *corev1.Namespace
		var testMoverImagePullSecrets string

		BeforeEach(func() {
			testMoverImagePullSecrets = ""

			isPrivileged = false
			userSuppliedSA = nil

			// Also create a test namespace to act as our volsync-controller namespace for this test
			testVolsyncSystemNamespace = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "volsync-system-for-sahandler-test-",
				},
			}
			Expect(k8sClient.Create(ctx, testVolsyncSystemNamespace)).To(Succeed())
			Expect(testVolsyncSystemNamespace.Name).NotTo(BeEmpty())
		})

		AfterEach(func() {
			// delete our test volsync-system ns
			Expect(k8sClient.Delete(ctx, testVolsyncSystemNamespace)).To(Succeed())
		})

		JustBeforeEach(func() {
			vsSAHandler, ok := saHandler.(*utils.SAHandlerVolSync)
			Expect(ok).To(BeTrue())

			// For testing only - fake out updating the volsyncSAHandler with our test volsync-system namespace
			// and MoverImagePullSecrets (The moverImagePullSecrets would normally come from cli params)
			vsSAHandler.VolSyncNamespace = testVolsyncSystemNamespace.GetName()
			vsSAHandler.PullSecretsMap = utils.ParseMoverImagePullSecrets(testMoverImagePullSecrets)
		})

		When("no image pull secrets are set", func() {
			BeforeEach(func() {
				testMoverImagePullSecrets = "" // This is the default, no image pull secrets set
			})

			It("should not copy any pull secrets to the mover ns", func() {
				var svcAccount *corev1.ServiceAccount
				Eventually(func() bool {
					var err error
					svcAccount, err = saHandler.Reconcile(ctx, logger)
					return svcAccount != nil && err == nil
				}, timeout, interval).Should(BeTrue())

				// No pull secrets should be set on the svcAccount
				Expect(len(svcAccount.ImagePullSecrets)).To(Equal(0))

				// Check no secrets copied to our mover (i.e replicationsource) namespace
				secretsList := &corev1.SecretList{}
				Expect(k8sClient.List(ctx, secretsList, client.InNamespace(namespace.GetName()))).To(Succeed())
				Expect(len(secretsList.Items)).To(Equal(0))
			})
		})

		When("An image pull secret is set", func() {
			secretName := "test-pull-1"
			BeforeEach(func() {
				testMoverImagePullSecrets = secretName
			})

			When("The secret does not exist in the volsync controller namespace", func() {
				It("Should not error and not copy the secret or update the svc account", func() {
					var svcAccount *corev1.ServiceAccount
					Eventually(func() bool {
						var err error
						svcAccount, err = saHandler.Reconcile(ctx, logger)
						return svcAccount != nil && err == nil
					}, timeout, interval).Should(BeTrue())

					// pull secret should still be set on the svcAccount
					Expect(len(svcAccount.ImagePullSecrets)).To(Equal(1))
					Expect(svcAccount.ImagePullSecrets).To(Equal([]corev1.LocalObjectReference{
						{
							Name: "volsync-pull-" + utils.GetHashedName(secretName),
						},
					}))

					// Check no secrets copied to our mover (i.e replicationsource) namespace
					secretsList := &corev1.SecretList{}
					Expect(k8sClient.List(ctx, secretsList, client.InNamespace(namespace.GetName()))).To(Succeed())
					Expect(len(secretsList.Items)).To(Equal(0))
				})
			})

			When("The secret exists in the volsync controller namespace", func() {
				var secret *corev1.Secret

				const origDockerConfigJSON string = `{
"auths": {
  "someplace.somewhere.com": {},
    "myrepo.nowheree.io": {
      "auth": "fakefakefake"
    }
  },
	"currentContext": "default"
}`

				BeforeEach(func() {
					secret = &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      secretName,
							Namespace: testVolsyncSystemNamespace.GetName(),
						},
						Type: corev1.SecretTypeDockerConfigJson,

						Data: map[string][]byte{
							".dockerconfigjson": []byte(origDockerConfigJSON),
						},
					}

					Expect(k8sClient.Create(ctx, secret)).To(Succeed())
				})

				expectedCopiedSecretName := "volsync-pull-" + utils.GetHashedName(secretName)

				JustBeforeEach(func() {
					var svcAccount *corev1.ServiceAccount
					Eventually(func() bool {
						var err error
						svcAccount, err = saHandler.Reconcile(ctx, logger)
						return svcAccount != nil && err == nil
					}, timeout, interval).Should(BeTrue())

					// pull secret should be set on the svcAccount
					Expect(len(svcAccount.ImagePullSecrets)).To(Equal(1))
					Expect(svcAccount.ImagePullSecrets).To(Equal([]corev1.LocalObjectReference{
						{
							Name: expectedCopiedSecretName,
						},
					}))

					// Check the secret is copied to our mover (i.e replicationsource) namespace
					secretsList := &corev1.SecretList{}
					Expect(k8sClient.List(ctx, secretsList, client.InNamespace(namespace.GetName()))).To(Succeed())
					Expect(len(secretsList.Items)).To(Equal(1))
					copiedSecret := secretsList.Items[0]
					Expect(copiedSecret.GetName()).To(Equal(expectedCopiedSecretName))
					Expect(copiedSecret.Type).To(Equal(secret.Type))
					Expect(copiedSecret.Data).To(Equal(secret.Data))

					// Also check that owner reference is set
					Expect(len(copiedSecret.OwnerReferences)).To(Equal(1))
					Expect(copiedSecret.OwnerReferences[0].Kind).To(Equal("ReplicationSource"))
					Expect(copiedSecret.OwnerReferences[0].Name).To(Equal(ownerRS.GetName()))
				})

				It("Should copy the pull secret to mover ns and update the svc account", func() {
					// Tests are in the JustBeforeEach()
				})

				When("The original pull secret is updated", func() {
					It("Should update the copied secret", func() {
						// Parent JustBeforeEach() has already reconciled the svc account via saHandler.Reconcile
						// Now update the secret data and reconcile again
						secret.Data[".dockerconfigjson"] = []byte("{\"a\":\"b\"}")
						secret.Data["extra-and-unnecessary"] = []byte("aaaa")
						Expect(k8sClient.Update(ctx, secret)).To(Succeed())

						// Reconcile again
						var svcAccount *corev1.ServiceAccount
						Eventually(func() bool {
							var err error
							svcAccount, err = saHandler.Reconcile(ctx, logger)
							return svcAccount != nil && err == nil
						}, timeout, interval).Should(BeTrue())

						// Everything else should still be set
						Expect(len(svcAccount.ImagePullSecrets)).To(Equal(1))
						// Check the secret is copied to our mover (i.e replicationsource) namespace
						secretsList := &corev1.SecretList{}
						Expect(k8sClient.List(ctx, secretsList, client.InNamespace(namespace.GetName()))).To(Succeed())
						Expect(len(secretsList.Items)).To(Equal(1))
						copiedSecret := secretsList.Items[0]
						Expect(copiedSecret.GetName()).To(Equal(expectedCopiedSecretName))
						Expect(copiedSecret.Type).To(Equal(secret.Type))
						Expect(copiedSecret.Data).To(Equal(secret.Data)) // Should be up-to-date
						Expect(len(copiedSecret.Data)).To(Equal(2))      // We put an extra k/v into the data
					})
				})

			})

		})
		When("Multiple image pull secrets are set", func() {
			secretNameA := "test-pull-a"
			secretNameB := "test-pull-b"
			secretNameC := "test-pull-c"

			var secretA, secretC *corev1.Secret
			BeforeEach(func() {
				testMoverImagePullSecrets = secretNameA + "," + secretNameB + "," + secretNameC

				// For these tests, will create secretA and C but not B
				// We should expect no failures, but secretB will not be copied
				secretA = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      secretNameA,
						Namespace: testVolsyncSystemNamespace.GetName(),
					},
					Type: corev1.SecretTypeDockerConfigJson,

					Data: map[string][]byte{
						".dockerconfigjson": []byte("{\"test\":\"value\"}"),
					},
				}
				Expect(k8sClient.Create(ctx, secretA)).To(Succeed())

				secretC = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      secretNameC,
						Namespace: testVolsyncSystemNamespace.GetName(),
					},
					Type: corev1.SecretTypeDockerConfigJson,

					Data: map[string][]byte{
						".dockerconfigjson": []byte("{\"different\":\"value\"}"),
					},
				}
				Expect(k8sClient.Create(ctx, secretC)).To(Succeed())

			})

			expectedCopiedSecretNameA := "volsync-pull-" + utils.GetHashedName(secretNameA)
			expectedCopiedSecretNameB := "volsync-pull-" + utils.GetHashedName(secretNameB)
			expectedCopiedSecretNameC := "volsync-pull-" + utils.GetHashedName(secretNameC)

			var copiedSecretA, copiedSecretC *corev1.Secret

			JustBeforeEach(func() {
				var svcAccount *corev1.ServiceAccount
				Eventually(func() bool {
					var err error
					svcAccount, err = saHandler.Reconcile(ctx, logger)
					return svcAccount != nil && err == nil
				}, timeout, interval).Should(BeTrue())

				// pull secrets should be set on the svcAccount
				Expect(len(svcAccount.ImagePullSecrets)).To(Equal(3))
				Expect(svcAccount.ImagePullSecrets).To(ContainElement(
					corev1.LocalObjectReference{
						Name: expectedCopiedSecretNameA,
					},
				))
				Expect(svcAccount.ImagePullSecrets).To(ContainElement(
					corev1.LocalObjectReference{
						Name: expectedCopiedSecretNameB,
					},
				))
				Expect(svcAccount.ImagePullSecrets).To(ContainElement(
					corev1.LocalObjectReference{
						Name: expectedCopiedSecretNameC,
					},
				))

				// Check the secrets (only A and C since B does not exist) are copied to our mover namespace
				secretsList := &corev1.SecretList{}
				Expect(k8sClient.List(ctx, secretsList, client.InNamespace(namespace.GetName()))).To(Succeed())
				Expect(len(secretsList.Items)).To(Equal(2))

				for i := range secretsList.Items {
					switch secretsList.Items[i].Name {
					case expectedCopiedSecretNameA:
						copiedSecretA = &secretsList.Items[i]
					case expectedCopiedSecretNameC:
						copiedSecretC = &secretsList.Items[i]
					}
				}
				Expect(copiedSecretA).NotTo(BeNil())
				Expect(copiedSecretC).NotTo(BeNil())

				Expect(copiedSecretA.Type).To(Equal(secretA.Type))
				Expect(copiedSecretA.Data).To(Equal(secretA.Data))
				Expect(len(copiedSecretA.OwnerReferences)).To(Equal(1))
				Expect(copiedSecretA.OwnerReferences[0].Kind).To(Equal("ReplicationSource"))
				Expect(copiedSecretA.OwnerReferences[0].Name).To(Equal(ownerRS.GetName()))

				Expect(copiedSecretC.Type).To(Equal(secretC.Type))
				Expect(copiedSecretC.Data).To(Equal(secretC.Data))
				Expect(len(copiedSecretC.OwnerReferences)).To(Equal(1))
				Expect(copiedSecretC.OwnerReferences[0].Kind).To(Equal("ReplicationSource"))
				Expect(copiedSecretC.OwnerReferences[0].Name).To(Equal(ownerRS.GetName()))
			})

			It("Should copy the secrets to the mover namespace and update the service account", func() {
				// See JustBeforeEach for most of the validations

				// Check owner references, there should be just 1
				Expect(copiedSecretA.Type).To(Equal(secretA.Type))
				Expect(copiedSecretA.Data).To(Equal(secretA.Data))
				Expect(len(copiedSecretA.OwnerReferences)).To(Equal(1))
				Expect(copiedSecretA.OwnerReferences[0].Kind).To(Equal("ReplicationSource"))
				Expect(copiedSecretA.OwnerReferences[0].Name).To(Equal(ownerRS.GetName()))

				Expect(copiedSecretC.Type).To(Equal(secretC.Type))
				Expect(copiedSecretC.Data).To(Equal(secretC.Data))
				Expect(len(copiedSecretC.OwnerReferences)).To(Equal(1))
				Expect(copiedSecretC.OwnerReferences[0].Kind).To(Equal("ReplicationSource"))
				Expect(copiedSecretC.OwnerReferences[0].Name).To(Equal(ownerRS.GetName()))

			})

			When("Multiple movers are in the same namespace", func() {
				// Make another owner CR (replicationdest, why not) and SAHandler to represent the other mover
				var ownerRD *volsyncv1alpha1.ReplicationDestination
				var saHandler2 utils.SAHandler

				BeforeEach(func() {
					ownerRD = &volsyncv1alpha1.ReplicationDestination{
						ObjectMeta: metav1.ObjectMeta{
							GenerateName: "rd-test-",
							Namespace:    namespace.GetName(),
						},
						Spec: volsyncv1alpha1.ReplicationDestinationSpec{},
					}
					Expect(k8sClient.Create(ctx, ownerRD)).To(Succeed())
				})

				JustBeforeEach(func() {
					// Initialize the 2nd saHAndler
					saHandler2 = utils.NewSAHandler(k8sClient, ownerRD, false, isPrivileged, nil)

					vsSAHandler2, ok := saHandler2.(*utils.SAHandlerVolSync)
					Expect(ok).To(BeTrue())

					// For testing only - fake out updating the volsyncSAHandler with our test volsync-system namespace
					// and MoverImagePullSecrets (The moverImagePullSecrets would normally come from cli params)
					vsSAHandler2.VolSyncNamespace = testVolsyncSystemNamespace.GetName()
					vsSAHandler2.PullSecretsMap = utils.ParseMoverImagePullSecrets(testMoverImagePullSecrets)
				})

				It("each mover/sahandler should update its own svc account and copied pull secrets should be shared", func() {
					var svcAccount1, svcAccount2 *corev1.ServiceAccount
					Eventually(func() bool {
						var err1, err2 error

						// Run both reconcilers
						svcAccount1, err1 = saHandler.Reconcile(ctx, logger)
						svcAccount2, err2 = saHandler2.Reconcile(ctx, logger)
						return svcAccount1 != nil && err1 == nil && svcAccount2 != nil && err2 == nil
					}, timeout, interval).Should(BeTrue())

					// pull secrets should be set on both svcAccounts
					for _, svcAcct := range []*corev1.ServiceAccount{svcAccount1, svcAccount2} {
						Expect(len(svcAcct.ImagePullSecrets)).To(Equal(3))
						Expect(svcAcct.ImagePullSecrets).To(ContainElement(
							corev1.LocalObjectReference{
								Name: expectedCopiedSecretNameA,
							},
						))
						Expect(svcAcct.ImagePullSecrets).To(ContainElement(
							corev1.LocalObjectReference{
								Name: expectedCopiedSecretNameB,
							},
						))
						Expect(svcAcct.ImagePullSecrets).To(ContainElement(
							corev1.LocalObjectReference{
								Name: expectedCopiedSecretNameC,
							},
						))
					}

					// Check the secrets (only A and C since B does not exist) are copied to our mover namespace
					secretsList := &corev1.SecretList{}
					Expect(k8sClient.List(ctx, secretsList, client.InNamespace(namespace.GetName()))).To(Succeed())
					Expect(len(secretsList.Items)).To(Equal(2))

					for i := range secretsList.Items {
						switch secretsList.Items[i].Name {
						case expectedCopiedSecretNameA:
							copiedSecretA = &secretsList.Items[i]
						case expectedCopiedSecretNameC:
							copiedSecretC = &secretsList.Items[i]
						}
					}
					Expect(copiedSecretA).NotTo(BeNil())
					Expect(copiedSecretC).NotTo(BeNil())

					Expect(copiedSecretA.Type).To(Equal(secretA.Type))
					Expect(copiedSecretA.Data).To(Equal(secretA.Data))
					Expect(copiedSecretC.Type).To(Equal(secretC.Type))
					Expect(copiedSecretC.Data).To(Equal(secretC.Data))

					// Secrets should have owner references for both owner CRs
					Expect(len(copiedSecretA.OwnerReferences)).To(Equal(2))
					Expect(copiedSecretA.OwnerReferences).To(ContainElement(
						gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
							"Kind": Equal("ReplicationSource"),
							"Name": Equal(ownerRS.GetName()),
						}),
					))
					Expect(copiedSecretA.OwnerReferences).To(ContainElement(
						gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
							"Kind": Equal("ReplicationDestination"),
							"Name": Equal(ownerRD.GetName()),
						}),
					))

					Expect(len(copiedSecretC.OwnerReferences)).To(Equal(2))
					Expect(copiedSecretC.OwnerReferences).To(ContainElement(
						gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
							"Kind": Equal("ReplicationSource"),
							"Name": Equal(ownerRS.GetName()),
						}),
					))
					Expect(copiedSecretC.OwnerReferences).To(ContainElement(
						gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
							"Kind": Equal("ReplicationDestination"),
							"Name": Equal(ownerRD.GetName()),
						}),
					))
				})
			})
		})
	})
})
