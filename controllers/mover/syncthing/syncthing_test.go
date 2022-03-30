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
package syncthing

import (
	"context"
	"strconv"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/controllers/mover"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var _ = Describe("Syncthing properly registers", func() {
	When("Syncthing's registration function is called", func() {
		BeforeEach(func() {
			// register syncthing module with movers
			Expect(Register()).To(Succeed())
		})

		It("is added to the mover catalog", func() {
			// nothing else is registered so found == syncthing registered
			found := false
			for _, v := range mover.Catalog {
				if v.(*Builder) != nil {
					found = true
				}
			}
			Expect(found).To(BeTrue())
		})
	})
})

var _ = Describe("Syncthing ignores other movers", func() {
	var logger = zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))

	It("An RS isn't for syncthing", func() {
		// create simple RS with explicit nil for Syncthing
		rs := &volsyncv1alpha1.ReplicationSource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rs-test",
				Namespace: "default",
			},
			Spec: volsyncv1alpha1.ReplicationSourceSpec{
				Syncthing: nil,
			},
		}

		// ensures that nothing happens
		mover, err := commonBuilderForTestSuite.FromSource(k8sClient, logger, rs)
		Expect(mover).To(BeNil())
		Expect(err).NotTo(HaveOccurred())
	})
})

var _ = Describe("Syncthing doesn't implement RD", func() {
	var ctx = context.TODO()
	var ns *corev1.Namespace
	logger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))
	var rd *volsyncv1alpha1.ReplicationDestination

	When("An RD Specifies Syncthing", func() {
		BeforeEach(func() {

			// create a namespace for test
			ns = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		})

		It("error occurs", func() {
			// make sure that syncthing never works with ReplicationDestination
			rd = &volsyncv1alpha1.ReplicationDestination{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "syncthing-rd",
					Namespace: ns.Name,
				},
				Spec: volsyncv1alpha1.ReplicationDestinationSpec{},
			}

			// get a builder from the xRD to ensure that this errors
			m, e := commonBuilderForTestSuite.FromDestination(k8sClient, logger, rd)
			Expect(e).To(HaveOccurred())
			Expect(m).To(BeNil())
		})
	})
})

var _ = Describe("When an RS specifies Syncthing", func() {
	var ctx = context.TODO()
	var ns *corev1.Namespace
	logger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))
	var rs *volsyncv1alpha1.ReplicationSource
	var srcPVC *corev1.PersistentVolumeClaim
	var mover *Mover

	BeforeEach(func() {
		// create a namespace for each test
		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "syncthing-test-",
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())

		// create a source PVC to be used for syncthing
		srcPVC = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "syncthing-src-",
				Namespace:    ns.Name,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("1Gi"),
					},
				},
			},
		}

		// wait until the PVC is available
		Expect(k8sClient.Create(ctx, srcPVC)).To(Succeed())
		Eventually(func() error {
			pvc := &corev1.PersistentVolumeClaim{}
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(srcPVC), pvc)
			return err
		}, "10s", "1s").Should(Succeed())

		// create replicationsource
		rs = &volsyncv1alpha1.ReplicationSource{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "syncthing-rs-",
				Namespace:    ns.Name,
			},
			Spec: volsyncv1alpha1.ReplicationSourceSpec{
				SourcePVC: srcPVC.Name,
				Syncthing: &volsyncv1alpha1.ReplicationSourceSyncthingSpec{},
			},
		}
	})
	JustBeforeEach(func() {
		// launch the replicationsource once edits are complete
		Expect(k8sClient.Create(ctx, rs)).To(Succeed())
	})

	AfterEach(func() {
		// GC the resources created by this test
		Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
	})

	// test for clauses beyond providing a minimal syncthing spec
	When("syncthing is used", func() {
		JustBeforeEach(func() {
			// Set status for controller
			rs.Status = &volsyncv1alpha1.ReplicationSourceStatus{}

			// create a syncthing mover
			m, err := commonBuilderForTestSuite.FromSource(k8sClient, logger, rs)
			Expect(err).NotTo(HaveOccurred())
			Expect(m).NotTo(BeNil())

			// cast as a mover
			mover, _ = m.(*Mover)
			Expect(mover).NotTo(BeNil())
			Expect(mover.owner).NotTo(BeNil())
		})

		It("owner exists on mover", func() {
			Expect(mover.owner).To(Equal(rs))
		})

		// test that the mover works with ClusterIP and LoadBalancer
		Context("services are created properly", func() {
			var svcType corev1.ServiceType

			When("serviceType is ClusterIP", func() {
				BeforeEach(func() {
					// set the service type
					svcType = corev1.ServiceTypeClusterIP
					rs.Spec.Syncthing.ServiceType = &svcType
				})

				It("ClusterIP is created", func() {
					// make sure the service is created
					svc, err := mover.ensureDataService(ctx)
					Expect(err).NotTo(HaveOccurred())
					Expect(svc).NotTo(BeNil())
					Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
				})

				It("Can get DataServiceAddress", func() {
					// test data
					const staticIP string = "1.2.3.4"

					// simple clusterIP without address
					svc := &corev1.Service{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "syncthing-data",
							Namespace: ns.Name,
						},
						Spec: corev1.ServiceSpec{
							Type: corev1.ServiceTypeClusterIP,
						},
					}

					// no address = fail
					addr, e := mover.GetDataServiceAddress(svc)
					Expect(addr).To(Equal(""))
					Expect(e).To(HaveOccurred())

					// clusterIP with address must work
					svc.Spec.ClusterIP = staticIP
					addr, e = mover.GetDataServiceAddress(svc)
					Expect(addr).To(Equal("tcp://" + staticIP + ":" + strconv.Itoa(syncthingDataPort)))
					Expect(e).NotTo(HaveOccurred())
				})
			})

			When("serviceType is LoadBalancer", func() {
				BeforeEach(func() {
					// set the service type
					svcType = corev1.ServiceTypeLoadBalancer
					rs.Spec.Syncthing.ServiceType = &svcType
				})

				It("LoadBalancer is created", func() {
					// make sure the service is created
					svc, err := mover.ensureDataService(ctx)
					Expect(err).NotTo(HaveOccurred())
					Expect(svc).NotTo(BeNil())
					Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeLoadBalancer))
				})

				It("Can get DataServiceAddress", func() {
					// create an empty loadbalancer
					svc := &corev1.Service{
						ObjectMeta: metav1.ObjectMeta{
							GenerateName: "syncthing-data-",
							Namespace:    ns.Name,
						},
						Spec: corev1.ServiceSpec{
							Type: corev1.ServiceTypeLoadBalancer,
						},
					}

					// this should error
					address, e := mover.GetDataServiceAddress(svc)
					Expect(e).To(HaveOccurred())
					Expect(address).To(BeEmpty())

					// test data
					const staticIP string = "127.0.0.1"
					const staticHostName string = "george.costanza"

					// set a status with an ingress
					svc.Status = corev1.ServiceStatus{
						LoadBalancer: corev1.LoadBalancerStatus{
							Ingress: []corev1.LoadBalancerIngress{
								{
									IP: staticIP,
								},
							},
						},
					}

					// extract the address from the service
					address, e = mover.GetDataServiceAddress(svc)
					Expect(e).NotTo(HaveOccurred())
					Expect(address).To(Equal("tcp://" + staticIP + ":" + strconv.Itoa(syncthingDataPort)))

					// ensure address works when hostname is provided
					svc.Status.LoadBalancer.Ingress[0].Hostname = staticHostName
					address, e = mover.GetDataServiceAddress(svc)
					Expect(e).NotTo(HaveOccurred())
					Expect(address).To(Equal("tcp://" + staticHostName + ":" + strconv.Itoa(syncthingDataPort)))
				})
			})
		})

		Context("Cleanup is handled properly", func() {
			// resources created by Syncthing
			// var apiSvc *corev1.Service
			// var dataSvc *corev1.Service
			// var apiSecret *corev1.Secret
			// var configPVC *corev1.PersistentVolumeClaim

			// BeforeEach(func() {

			// })
		})

		Context("dataPVC is provided", func() {

			It("mover ensures PVC is available or fails", func() {
				// when a real PVC is provided
				returnedPVC, err := mover.ensureDataPVC(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(returnedPVC).NotTo(BeNil())

				// create a fake PVC
				var nonexistentPVCName string = "this-name-clearly-doesnt-exist"
				mover.dataPVCName = &nonexistentPVCName
				returnedPVC, err = mover.ensureDataPVC(ctx)
				Expect(err).To(HaveOccurred())
				Expect(returnedPVC).To(BeNil())
			})
		})

		Context("VolSync ensures a config PVC", func() {
			var configPVC *corev1.PersistentVolumeClaim

			When("configPVC already exists", func() {
				JustBeforeEach(func() {
					// create a config PVC
					configPVC = &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "volsync-" + mover.owner.GetName() + "-config",
							Namespace: ns.Name,
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							AccessModes: []corev1.PersistentVolumeAccessMode{
								corev1.ReadWriteOnce,
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: resource.MustParse("1Gi"),
								},
							},
						},
					}
					Expect(k8sClient.Create(ctx, configPVC)).To(Succeed())
					Eventually(func() error {
						return k8sClient.Get(ctx, types.NamespacedName{Name: configPVC.Name, Namespace: ns.Name}, configPVC)
					}).Should(Succeed())
				})

				It("VolSync uses existing configPVC", func() {
					// volsync reuses existing PVC
					returnedPVC, err := mover.ensureConfigPVC(ctx)
					Expect(err).NotTo(HaveOccurred())
					Expect(returnedPVC).NotTo(BeNil())
					Expect(returnedPVC.Name).To(Equal(configPVC.Name))
				})
			})

			When("configPVC doesn't exist", func() {
				It("VolSync creates a new configPVC", func() {
					// volsync creates & returns a config PVC
					configPVC, err := mover.ensureConfigPVC(ctx)
					Expect(err).NotTo(HaveOccurred())
					Expect(configPVC).NotTo(BeNil())

					// ensure the PVC is created
					Eventually(func() error {
						return k8sClient.Get(ctx, types.NamespacedName{Name: configPVC.Name, Namespace: configPVC.Namespace}, configPVC)
					}, timeout, interval).Should(Succeed())
				})
			})
		})

		Context("validate apikey secret", func() {
			var apiKeys *corev1.Secret

			When("secret already exists", func() {
				JustBeforeEach(func() {
					// create the secret
					apiKeys = &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "volsync-" + mover.owner.GetName(),
							Namespace: ns.Name,
						},
						Data: map[string][]byte{
							"apikey":   []byte("my-secret-apikey-do-not-steal"),
							"username": []byte("gcostanza"),
							"password": []byte("bosco"),
						},
					}
					Expect(k8sClient.Create(ctx, apiKeys)).To(Succeed())
					Eventually(func() error {
						return k8sClient.Get(ctx, types.NamespacedName{Name: apiKeys.Name, Namespace: ns.Name}, apiKeys)
					}, timeout, interval).Should(Succeed())
				})

				It("VolSync retrieves secret", func() {
					// retrieve the secret
					returnedSecret, err := mover.ensureSecretAPIKey(ctx)
					Expect(err).NotTo(HaveOccurred())
					Expect(returnedSecret).NotTo(BeNil())
					Expect(returnedSecret.Name).To(Equal(apiKeys.Name))

					// ensure the data keys exist
					Expect(returnedSecret.Data["apikey"]).NotTo(BeNil())
					Expect(returnedSecret.Data["username"]).NotTo(BeNil())
					Expect(returnedSecret.Data["password"]).NotTo(BeNil())
				})
			})

			When("VolSync creates the secret", func() {
				It("VolSync creates the secret", func() {
					// create the secret
					returnedSecret, err := mover.ensureSecretAPIKey(ctx)
					Expect(err).NotTo(HaveOccurred())
					Expect(returnedSecret).NotTo(BeNil())

					// ensure the secret is created
					Eventually(func() error {
						return k8sClient.Get(ctx, types.NamespacedName{Name: returnedSecret.Name, Namespace: returnedSecret.Namespace}, returnedSecret)
					}, timeout, interval).Should(Succeed())

					// ensure the data keys exist
					Expect(returnedSecret.Data["apikey"]).NotTo(BeNil())
					Expect(returnedSecret.Data["apikey"]).NotTo(BeEmpty())
					Expect(returnedSecret.Data["username"]).NotTo(BeNil())
					Expect(returnedSecret.Data["username"]).NotTo(BeEmpty())
					Expect(returnedSecret.Data["password"]).NotTo(BeNil())
					Expect(returnedSecret.Data["password"]).NotTo(BeEmpty())

				})
			})
		})

		Context("Syncthing API is being used properly", func() {
			// todo
		})

		Context("service account is created", func() {
			It("creates a new one", func() {
				// create the SA
				sa, err := mover.ensureSA(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(sa).NotTo(BeNil())

				// make sure that the SA exists in-cluster
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{Name: sa.Name, Namespace: ns.Name}, sa)
				}, timeout, interval).Should(Succeed())
			})
		})

		Context("Syncthing is being properly used", func() {
			var configPVC *corev1.PersistentVolumeClaim
			var apiSecret *corev1.Secret
			var sa *corev1.ServiceAccount
			var deployment *appsv1.Deployment
			var apiService *corev1.Service

			JustBeforeEach(func() {
				var err error

				// create necessary elements
				configPVC = &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "volsync-" + mover.owner.GetName(),
						Namespace: ns.Name,
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse("1Gi"),
							},
						},
					},
				}

				// expect the PVC to be created
				Expect(k8sClient.Create(ctx, configPVC)).To(Succeed())

				// create the apiSecret
				apiSecret = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "volsync-" + mover.owner.GetName(),
						Namespace: ns.Name,
					},
					Data: map[string][]byte{
						"apikey":   []byte("my-secret-apikey-do-not-steal"),
						"username": []byte("gcostanza"),
						"password": []byte("bosco"),
					},
				}
				Expect(k8sClient.Create(ctx, apiSecret)).To(Succeed())

				// expect the resources to have been created by now
				Eventually(func() error {
					err := k8sClient.Get(ctx, types.NamespacedName{Name: configPVC.Name, Namespace: ns.Name}, configPVC)
					if err != nil {
						return err
					}

					err = k8sClient.Get(ctx, types.NamespacedName{Name: apiSecret.Name, Namespace: ns.Name}, apiSecret)
					if err != nil {
						return err
					}
					return nil
				}, timeout, interval).Should(Succeed())

				// ensure the SA exists
				sa, err = mover.ensureSA(ctx)
				Expect(sa).NotTo(BeNil())
				Expect(err).NotTo(HaveOccurred())

				// ensure the SA exists in-cluster
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{Name: sa.Name, Namespace: ns.Name}, sa)
				}, timeout, interval).Should(Succeed())
			})

			When("VolSync ensures a deployment", func() {
				It("creates a new one", func() {
					// create a deployment
					deployment, err := mover.ensureDeployment(ctx, srcPVC, configPVC, sa, apiSecret)
					Expect(err).NotTo(HaveOccurred())
					Expect(deployment).NotTo(BeNil())

					// expect deployment to have syncthing container
					Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
					stContainer := deployment.Spec.Template.Spec.Containers[0]
					Expect(stContainer.Name).To(Equal("syncthing"))

					// make sure the mover's containerImage was specified for stContainer
					Expect(stContainer.Image).To(Equal(mover.containerImage))

					// expect STGUIAPIKEY to be set as one of the envs & referencing the secret
					Expect(stContainer.Env).To(HaveLen(3))
					for _, env := range stContainer.Env {
						if env.Name == "STGUIAPIKEY" {
							Expect(env.ValueFrom.SecretKeyRef.Name).To(Equal(apiSecret.Name))
							Expect(env.ValueFrom.SecretKeyRef.Key).To(Equal("apikey"))
						}
					}

					// validate that the deployment exposes API & data ports
					var portNames []string = []string{"api", "data"}

					// make sure the above ports exist in container's ports
					for _, port := range stContainer.Ports {
						found := false
						for _, name := range portNames {
							if port.Name == name {
								found = true
								break
							}
						}
						// portname should be found in the list
						Expect(found).To(BeTrue())
					}

					// make sure that configPVC and srcPVC are referenced in the deployment
					Expect(deployment.Spec.Template.Spec.Volumes).To(HaveLen(2))
					for _, volume := range deployment.Spec.Template.Spec.Volumes {
						if volume.Name == "syncthing-config" {
							Expect(volume.PersistentVolumeClaim.ClaimName).To(Equal(configPVC.Name))
						} else if volume.Name == "syncthing-data" {
							Expect(volume.PersistentVolumeClaim.ClaimName).To(Equal(srcPVC.Name))
						}
					}

					// make sure that both deployment's specified volumes are being mounted by the container
					Expect(stContainer.VolumeMounts).To(HaveLen(2))
					for _, mount := range stContainer.VolumeMounts {
						if mount.Name == "syncthing-config" {
							Expect(mount.MountPath).To(Equal("/config"))
						} else if mount.Name == "syncthing-data" {
							Expect(mount.MountPath).To(Equal("/data"))
						}
					}
				})

				When("Deployment already exists", func() {
					JustBeforeEach(func() {
						// need to declare this err so scoped deployment can be used in test
						var err error

						// create a deployment
						deployment, err = mover.ensureDeployment(ctx, srcPVC, configPVC, sa, apiSecret)
						Expect(err).NotTo(HaveOccurred())
						Expect(deployment).NotTo(BeNil())

						// await the avalability of the deployment
						Eventually(func() error {
							return k8sClient.Get(ctx, types.NamespacedName{Name: deployment.Name, Namespace: ns.Name}, deployment)
						}, timeout, interval).Should(Succeed())
					})

					It("reuses the running deployment", func() {
						// retrieve the deployment
						newDeployment, err := mover.ensureDeployment(ctx, srcPVC, configPVC, sa, apiSecret)
						Expect(err).NotTo(HaveOccurred())
						Expect(newDeployment).NotTo(BeNil())

						// ensure the deployment is the same
						Expect(newDeployment.Name).To(Equal(deployment.Name))
					})

					When("Service exposes the deployment's API", func() {
						JustBeforeEach(func() {
							// get the deployment
							deployment, err := mover.ensureDeployment(ctx, srcPVC, configPVC, sa, apiSecret)
							Expect(err).NotTo(HaveOccurred())
							Expect(deployment).NotTo(BeNil())

							// ensure the API service
							apiService, err = mover.ensureAPIService(ctx, deployment)
							Expect(err).NotTo(HaveOccurred())
							Expect(apiService).NotTo(BeNil())

							// get the service from the cluster
							Eventually(func() error {
								return k8sClient.Get(ctx, types.NamespacedName{Name: apiService.Name, Namespace: ns.Name}, apiService)
							}, timeout, interval).Should(Succeed())
						})

						It("is properly configured", func() {
							// get the deployment
							deployment, err := mover.ensureDeployment(ctx, srcPVC, configPVC, sa, apiSecret)
							Expect(err).NotTo(HaveOccurred())
							Expect(deployment).NotTo(BeNil())

							// ensure the API service
							apiService, err = mover.ensureAPIService(ctx, deployment)
							Expect(err).NotTo(HaveOccurred())
							Expect(apiService).NotTo(BeNil())

							// get the service from the cluster
							Eventually(func() error {
								return k8sClient.Get(ctx, types.NamespacedName{Name: apiService.Name, Namespace: ns.Name}, apiService)
							}, timeout, interval).Should(Succeed())

							// ensure that the service's ports match the deployments
							Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
							syncthingContainer := deployment.Spec.Template.Spec.Containers[0]

							// ensure that the deployment's API port matches the service's
							Expect(apiService.Spec.Ports).To(HaveLen(1))

							// find the API port
							var apiPort *corev1.ContainerPort
							for _, port := range syncthingContainer.Ports {
								if port.Name == "api" {
									apiPort = &port
									break
								}
							}
							Expect(apiService.Spec.Ports[0].Port).To(Equal(apiPort.ContainerPort))

							// API Service gets reused
							newApiService, err := mover.ensureAPIService(ctx, deployment)
							Expect(err).NotTo(HaveOccurred())
							Expect(newApiService).NotTo(BeNil())
							Expect(newApiService.ObjectMeta.Name).To(Equal(apiService.ObjectMeta.Name))
						})
					})
				})
			})
		})
	})

	It("mover is successfully created", func() {
		// set the status in lieu of controller
		rs.Status = &volsyncv1alpha1.ReplicationSourceStatus{}

		// make sure that builder is successfully retrieved
		m, err := commonBuilderForTestSuite.FromSource(k8sClient, logger, rs)
		Expect(err).ToNot(HaveOccurred())
		Expect(m).ToNot(BeNil())
	})
})
