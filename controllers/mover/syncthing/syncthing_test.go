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
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"time"

	"github.com/backube/volsync/api/v1alpha1"
	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/controllers/mover"
	cMover "github.com/backube/volsync/controllers/mover"
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

// TODO: add tests for the HTTPS certificates

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

	// 1
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

	// 3
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

		It("returns a name", func() {
			name := mover.Name()
			Expect(name).NotTo(BeEmpty())
			Expect(name).To(Equal("syncthing"))
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
			When("cleanup is called", func() {
				It("always succeeds", func() {
					Consistently(func() error {
						res, err := mover.Cleanup(ctx)
						if err != nil || res == cMover.InProgress() {
							return err
						}
						return nil
					}).Should(Succeed())
				})
			})
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

			JustBeforeEach(func() {
				// ensure the data PVC before testing the config PVC so the volumehandler is set
				_, err := mover.ensureDataPVC(ctx)
				Expect(err).NotTo(HaveOccurred())
			})

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
					returnedPVC, err := mover.ensureConfigPVC(ctx, srcPVC)
					Expect(err).NotTo(HaveOccurred())
					Expect(returnedPVC).NotTo(BeNil())
					Expect(returnedPVC.Name).To(Equal(configPVC.Name))
				})
			})

			When("configPVC doesn't exist", func() {
				It("VolSync creates a new configPVC", func() {
					// volsync creates & returns a config PVC
					configPVC, err := mover.ensureConfigPVC(ctx, srcPVC)
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
						return k8sClient.Get(ctx, types.NamespacedName{
							Name:      returnedSecret.Name,
							Namespace: returnedSecret.Namespace,
						}, returnedSecret)
					}, timeout, interval).Should(Succeed())

					// ensure that all of the required keys exist
					requiredKeys := []string{"apikey", "username", "password", "httpsCertPEM", "httpsKeyPEM"}
					for _, key := range requiredKeys {
						Expect(returnedSecret.Data[key]).NotTo(BeNil())
						Expect(returnedSecret.Data[key]).NotTo(BeEmpty())
					}
				})
			})
		})

		Context("Syncthing API is being used properly", func() {
			When("Syncthing server does not exist", func() {
				It("Doesn't synchronize", func() {
					res, err := mover.Synchronize(ctx)
					Expect(err).ToNot(BeNil())
					Expect(res).To(Equal(cMover.InProgress()))

					err = mover.syncthing.FetchSyncthingConfig()
					Expect(err).ToNot(BeNil())
					Expect(err.Error()).To(ContainSubstring("Get"))

					err = mover.syncthing.FetchSyncthingSystemStatus()
					Expect(err).ToNot(BeNil())
					Expect(err.Error()).To(ContainSubstring("Get"))

					err = mover.syncthing.FetchConnectedStatus()
					Expect(err).ToNot(BeNil())
					Expect(err.Error()).To(ContainSubstring("Get"))

					apiKeys := &corev1.Secret{
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

					err = mover.ensureIsConfigured(apiKeys)
					Expect(err).ToNot(BeNil())
					Expect(err.Error()).To(ContainSubstring("no such host"))
					Expect(err.Error()).To(ContainSubstring("Get"))

					service := &corev1.Service{
						ObjectMeta: metav1.ObjectMeta{
							Name:      mover.getAPIServiceName(),
							Namespace: mover.owner.GetNamespace(),
						},
					}
					err = mover.ensureStatusIsUpdated(service)
					Expect(err).ToNot(BeNil())
					Expect(err.Error()).To(ContainSubstring("no such host"))
					Expect(err.Error()).To(ContainSubstring("Get"))

					_, err = mover.syncthing.jsonRequest("/rest/config", "GET", nil)
					Expect(err).ToNot(BeNil())
					Expect(err.Error()).To(ContainSubstring("no such host"))
					Expect(err.Error()).To(ContainSubstring("Get"))
				})
			})

			When("Syncthing server exists", func() {
				var ts *httptest.Server
				var s SyncthingConfig

				JustBeforeEach(func() {
					sConfig := SyncthingConfig{
						Version: 10,
					}
					sStatus := SystemStatus{
						MyID: "test",
					}

					sConnections := SystemConnections{
						Total: TotalStats{At: "test"},
					}

					ts = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						switch r.URL.Path {
						case "/rest/config":
							if r.Method == "GET" {
								res := sConfig
								resBytes, _ := json.Marshal(res)
								fmt.Fprintln(w, string(resBytes))
							} else if r.Method == "PUT" {
								err := json.NewDecoder(r.Body).Decode(&s)
								if err != nil {
									http.Error(w, "Error decoding request body", http.StatusBadRequest)
									return
								}
							}
							return
						case "/rest/system/status":
							res := sStatus
							resBytes, _ := json.Marshal(res)
							fmt.Fprintln(w, string(resBytes))
							return
						case "/rest/system/connections":
							res := sConnections
							resBytes, _ := json.Marshal(res)
							fmt.Fprintln(w, string(resBytes))
							return
						default:
							return
						}
					}))

					mover.syncthing.APIConfig.APIURL = ts.URL
					mover.syncthing.APIConfig.Client = ts.Client()
					mover.syncthing.APIConfig.APIKey = "test"
				})

				JustAfterEach(func() {
					ts.Close()
				})

				It("Fetches the Latest Info", func() {
					err := mover.syncthing.FetchLatestInfo()
					Expect(err).To(BeNil())
					Expect(mover.syncthing.Config.Version).To(Equal(10))
					Expect(mover.syncthing.SystemStatus.MyID).To(Equal("test"))
					Expect(mover.syncthing.SystemConnections.Total.At).To(Equal("test"))
				})

				It("Updates the Syncthing Config", func() {
					mover.syncthing.Config = &SyncthingConfig{
						Version: 9,
					}
					err := mover.syncthing.UpdateSyncthingConfig()
					Expect(err).To(BeNil())
					Expect(s.Version).To(Equal(9))
				})

				It("Ensures it's configured", func() {
					apiKeys := &corev1.Secret{
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

					mover.peerList = []volsyncv1alpha1.SyncthingPeer{
						{
							Address: "/tcp/127.0.0.1/22000",
							ID:      "peer1",
						},
						{
							Address: "/tcp/127.0.0.2/22000",
							ID:      "peer2",
						},
					}

					err := mover.ensureIsConfigured(apiKeys)
					Expect(err).To(BeNil())

					for i, peer := range mover.peerList {
						expected := SyncthingDevice{
							DeviceID:   peer.ID,
							Addresses:  []string{peer.Address},
							Name:       fmt.Sprintf("Syncthing Device Configured by Volsync: %v", peer.ID),
							Introducer: peer.Introducer,
						}
						Expect(s.Devices[i]).To(Equal(expected))
					}
				})

				It("Ensures the status is updated", func() {
					service := &corev1.Service{
						ObjectMeta: metav1.ObjectMeta{
							Name:      mover.getAPIServiceName(),
							Namespace: mover.owner.GetNamespace(),
						},
						Spec: corev1.ServiceSpec{
							ClusterIP: "0.0.0.0",
						},
					}

					mover.peerList = []volsyncv1alpha1.SyncthingPeer{
						{
							Address: "/tcp/127.0.0.1/22000",
							ID:      "peer1",
						},
						{
							Address: "/tcp/127.0.0.2/22000",
							ID:      "peer2",
						},
					}

					err := mover.ensureStatusIsUpdated(service)
					Expect(err).To(BeNil())

					for i, peer := range mover.status.Peers {
						p := volsyncv1alpha1.SyncthingPeer{
							Address: peer.Address,
							ID:      peer.ID,
						}
						Expect(mover.peerList[i]).To(Equal(p))
					}
				})

				It("jsonRequests without errors", func() {
					_, err := mover.syncthing.jsonRequest("/rest/config", "GET", nil)
					Expect(err).To(BeNil())

					_, err = mover.syncthing.jsonRequest("/rest/system/status", "GET", nil)
					Expect(err).To(BeNil())

					_, err = mover.syncthing.jsonRequest("/rest/system/connections", "GET", nil)
					Expect(err).To(BeNil())
				})

			})
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

			When("synchronize is called", func() {
				It("without api, it errors", func() {
					// mover synchronize all of the resources
					result, err := mover.Synchronize(ctx)

					// ensure the result is not nil
					Expect(result).NotTo(BeNil())
					Expect(result).To(Equal(cMover.InProgress()))

					// ensure the error is not nil
					Expect(err).NotTo(BeNil())
				})

				When("datapvcname is invalid", func() {

					It("errors out", func() {
						// mess up the datapvcname
						var newDataPVCName = "invalid-name"
						mover.dataPVCName = &newDataPVCName

						// try to synchronize
						res, err := mover.Synchronize(ctx)

						// result should be in progress
						Expect(res).NotTo(BeNil())
						Expect(res).To(Equal(cMover.InProgress()))

						// error should be not nil
						Expect(err).NotTo(BeNil())
					})
				})
			})
			When("VolSync ensures a deployment", func() {
				It("creates a new one", func() {
					// TODO: ensure HTTPS keys are here

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
					Expect(stContainer.Env).To(HaveLen(4))
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
					for _, volume := range deployment.Spec.Template.Spec.Volumes {
						if volume.Name == "syncthing-config" {
							Expect(volume.PersistentVolumeClaim.ClaimName).To(Equal(configPVC.Name))
						} else if volume.Name == "syncthing-data" {
							Expect(volume.PersistentVolumeClaim.ClaimName).To(Equal(srcPVC.Name))
						}
					}

					// make sure that both deployment's specified volumes are being mounted by the container
					for _, mount := range stContainer.VolumeMounts {
						if mount.Name == "syncthing-config" {
							Expect(mount.MountPath).To(Equal("/config"))
						} else if mount.Name == "syncthing-data" {
							Expect(mount.MountPath).To(Equal("/data"))
						} else if mount.Name == "syncthing-certs" {
							Expect(mount.MountPath).To(Equal("/certs"))
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
							var apiPort corev1.ContainerPort
							for _, port := range syncthingContainer.Ports {
								if port.Name == "api" {
									apiPort = port
									break
								}
							}
							Expect(apiService.Spec.Ports[0].Port).To(Equal(apiPort.ContainerPort))

							// API Service gets reused
							newAPIService, err := mover.ensureAPIService(ctx, deployment)
							Expect(err).NotTo(HaveOccurred())
							Expect(newAPIService).NotTo(BeNil())
							Expect(newAPIService.ObjectMeta.Name).To(Equal(apiService.ObjectMeta.Name))
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

// These tests describe the behavior of the Syncthing struct and ensures that its methods
// are working as expected.
var _ = Describe("Syncthing utils", func() {
	Context("Syncthing object is used", func() {
		var syncthing Syncthing
		logger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))

		// Configure each Syncthing object beforehand
		BeforeEach(func() {
			// initialize these pointer fields
			syncthing = Syncthing{
				Config:            &SyncthingConfig{},
				SystemConnections: &SystemConnections{},
				SystemStatus: &SystemStatus{
					MyID: string(sha256.New().Sum([]byte("my-secure-private-key"))),
				},
				logger: logger,
			}
		})

		When("devices are called to update", func() {
			BeforeEach(func() {
				// create a folder
				syncthing.Config.Folders = append(syncthing.Config.Folders, SyncthingFolder{
					ID:    string(sha256.New().Sum([]byte("festivus-files-1986"))),
					Label: "festivus-files",
				})
			})

			It("updates them based on the provided peerList", func() {
				// create a peer list
				peerList := []v1alpha1.SyncthingPeer{
					{
						ID:      "george-costanza",
						Address: "/ip4/127.0.0.1/tcp/22000/quic",
					},
					{
						ID:      "jerry-seinfeld",
						Address: "/ip4/192.168.1.1/tcp/22000/quic",
					},
				}

				// update the devices
				syncthing.UpdateDevices(peerList)

				// ensure that the devices are updated
				Expect(syncthing.Config.Devices).To(HaveLen(2))

				// ensure that we can discover all of the devices on the local object
				discovered := 0
				for _, device := range syncthing.Config.Devices {
					if device.DeviceID == "george-costanza" {
						discovered++
					}
					if device.DeviceID == "jerry-seinfeld" {
						discovered++
					}
				}

				// we should have found all of the devices
				Expect(discovered).To(Equal(len(syncthing.Config.Devices)))

				// folders should have been shared with the new peers
				for _, folder := range syncthing.Config.Folders {
					Expect(len(folder.Devices)).To(Equal(len(peerList)))
				}

				// pass an empty peer list to ensure that the devices are removed
				syncthing.UpdateDevices([]v1alpha1.SyncthingPeer{})

				// ensure that the devices are removed
				Expect(syncthing.Config.Devices).To(HaveLen(0))
				for _, folder := range syncthing.Config.Folders {
					Expect(folder.Devices).To(HaveLen(0))
				}
			})

			When("syncthing lists itself within the devices entries", func() {
				BeforeEach(func() {
					// make sure that the syncthing is listed in the connections
					syncthing.Config.Devices = append(syncthing.Config.Devices, SyncthingDevice{
						DeviceID:  syncthing.SystemStatus.MyID,
						Name:      "current Syncthing node",
						Addresses: []string{"/ip4/0.0.0.0/tcp/22000/quic"},
					})
				})

				It("retains the entry when updated against an empty peerlist", func() {
					// pass an empty peer list and make sure that we can still find the self device
					syncthing.UpdateDevices([]v1alpha1.SyncthingPeer{})
					Expect(syncthing.Config.Devices).To(HaveLen(1))
					Expect(syncthing.Config.Devices[0].DeviceID).To(Equal(syncthing.SystemStatus.MyID))

				})

				It("only reconfigures when other syncthing devices are provided", func() {
					// pass an empty peer list and make sure that Syncthing doesn't need to be reconfigured
					Expect(syncthing.NeedsReconfigure([]v1alpha1.SyncthingPeer{})).To(BeFalse())

					// pass a peer list containing self to ensure that Syncthing doesn't need to be reconfigured
					Expect(syncthing.NeedsReconfigure([]v1alpha1.SyncthingPeer{
						{
							ID:      syncthing.SystemStatus.MyID,
							Address: "/ip4/127.0.0.1/tcp/22000/quic",
						},
					})).To(BeFalse())

					// specify a peer
					Expect(syncthing.NeedsReconfigure([]v1alpha1.SyncthingPeer{
						{
							ID:      "elaine-benes",
							Address: "/ip6/::1/tcp/22000/quic",
						},
					})).To(BeTrue())
				})
			})

			When("syncthing has an empty device list", func() {
				BeforeEach(func() {
					// clear Syncthing's device list and make sure that we can still find the self device
					syncthing.Config.Devices = []SyncthingDevice{}
				})

				It("adding itself has no effect", func() {
					// pass in a peerList containing an entry with our own ID
					syncthing.UpdateDevices([]v1alpha1.SyncthingPeer{
						{
							ID:      syncthing.SystemStatus.MyID,
							Address: "/ip6/::1/tcp/22000/quic",
						},
					})
					Expect(syncthing.Config.Devices).To(HaveLen(0))
				})

				It("only reconfigures when other syncthing devices are provided", func() {
					// test with an empty list
					peerList := []v1alpha1.SyncthingPeer{}
					needsReconfigure := syncthing.NeedsReconfigure(peerList)
					Expect(needsReconfigure).To(BeFalse())

					// specify ourself as a peer
					peerList = []v1alpha1.SyncthingPeer{
						{
							ID:      syncthing.SystemStatus.MyID,
							Address: "/ip6/::1/tcp/22000/quic",
						},
					}
					needsReconfigure = syncthing.NeedsReconfigure(peerList)
					Expect(needsReconfigure).To(BeFalse())

					// specify a peer
					peerList = []v1alpha1.SyncthingPeer{
						{
							ID:      "elaine-benes",
							Address: "/ip6/::1/tcp/22000/quic",
						},
					}
					needsReconfigure = syncthing.NeedsReconfigure(peerList)
					Expect(needsReconfigure).To(BeTrue())
				})
			})

			When("other devices are configured", func() {
				BeforeEach(func() {
					syncthingPeers := []v1alpha1.SyncthingPeer{
						{
							ID:      "george-costanzas-parents-house",
							Address: "/ip6/::1/tcp/22000/quic",
						},
						{
							ID:      "elaine-benes",
							Address: "/ip6/::1/tcp/22000/quic",
						},
						{
							ID:      "frank-costanza",
							Address: "/ip6/::1/tcp/22000/quic",
						},
					}
					syncthing.UpdateDevices(syncthingPeers)
				})

				It("doesn't reconfigure when no new devices are passed in", func() {
					// create a peerlist with the same devices as the current config
					peerList := []v1alpha1.SyncthingPeer{
						{
							ID:      "george-costanzas-parents-house",
							Address: "/ip6/::1/tcp/22000/quic",
						},
						{
							ID:      "elaine-benes",
							Address: "/ip6/::1/tcp/22000/quic",
						},
						{
							ID:      "frank-costanza",
							Address: "/ip6/::1/tcp/22000/quic",
						},
					}

					// Syncthing should not reconfigure when the peerlist is the same
					needsReconfigure := syncthing.NeedsReconfigure(peerList)
					Expect(needsReconfigure).To(BeFalse())

					// make sure that nothing new is updated
					previousDevices := syncthing.Config.Devices
					previousFolders := syncthing.Config.Folders
					syncthing.UpdateDevices(peerList)

					// make sure that all of the previous devices are the same as the current devices
					found := 0
					for _, device := range syncthing.Config.Devices {
						foundOne := false
						for _, previousDevice := range previousDevices {
							if device.DeviceID == previousDevice.DeviceID {
								found++
								foundOne = true
								break
							}
						}
						Expect(foundOne).To(BeTrue())
					}
					Expect(found).To(Equal(len(syncthing.Config.Devices)))

					// expect to find all of the current devices in the folder in the previous folder's list
					found = 0
					for _, device := range syncthing.Config.Folders[0].Devices {
						foundOne := false
						for _, previousDevice := range previousFolders[0].Devices {
							if device.DeviceID == previousDevice.DeviceID {
								found++
								foundOne = true
								break
							}
						}
						Expect(foundOne).To(BeTrue())
					}
					Expect(found).To(Equal(len(syncthing.Config.Folders[0].Devices)))
				})

				It("only needs reconfigure when the list differs but ignores the self syncthing device", func() {
					// test with an empty list
					peerList := []v1alpha1.SyncthingPeer{}
					needsReconfigure := syncthing.NeedsReconfigure(peerList)
					Expect(needsReconfigure).To(BeTrue())

					// Syncthing should view this as erasing all peers
					peerList = []v1alpha1.SyncthingPeer{
						{
							ID:      syncthing.SystemStatus.MyID,
							Address: "/ip6/::1/tcp/22000/quic",
						},
					}
					Expect(syncthing.NeedsReconfigure(peerList)).To(BeTrue())
				})

				It("can reconfigure to a larger list", func() {
					// create a peerlist based on the configured devices in the Syncthing object
					replicaPeerList := []v1alpha1.SyncthingPeer{}
					for _, device := range syncthing.Config.Devices {
						replicaPeerList = append(replicaPeerList, v1alpha1.SyncthingPeer{
							ID:      device.DeviceID,
							Address: device.Addresses[0],
						})
					}
					// specify an additional peer
					replicaPeerList = append(replicaPeerList,
						v1alpha1.SyncthingPeer{
							ID:      "kramers-apartment",
							Address: "/ip4/256.256.256.256/tcp/22000/quic",
						},
					)
					Expect(syncthing.NeedsReconfigure(replicaPeerList)).To(BeTrue())

					// update the Syncthing config with the new peerlist
					syncthing.UpdateDevices(replicaPeerList)

					// expect to find the new peer in the config
					found := false
					for _, device := range syncthing.Config.Devices {
						found = device.DeviceID == "kramers-apartment"
						if found {
							break
						}
					}
					Expect(found).To(BeTrue())

					// expect to find the new peer in the folder
					found = false
					for _, device := range syncthing.Config.Folders[0].Devices {
						found = device.DeviceID == "kramers-apartment"
						if found {
							break
						}
					}
					Expect(found).To(BeTrue())
				})

				It("reconfigures to a smaller list", func() {
					// create a smaller peerlist with a subset of the devices in the current config
					peerList := []v1alpha1.SyncthingPeer{
						{
							ID:      "george-costanzas-parents-house",
							Address: "/ip6/::1/tcp/22000/quic",
						},
						{
							ID:      "elaine-benes",
							Address: "/ip6/::1/tcp/22000/quic",
						},
						{
							ID:      "frank-costanza",
							Address: "/ip6/::1/tcp/22000/quic",
						},
					}
					peerToRemove := peerList[1]

					// only specify a subset of the peers
					peerListSubset := []v1alpha1.SyncthingPeer{}
					for _, peer := range peerList {
						if peer.ID != peerToRemove.ID {
							peerListSubset = append(peerListSubset, peer)
						}
					}

					// syncthing must reconfigure to exclude the missing peer
					Expect(syncthing.NeedsReconfigure(peerListSubset)).To(BeTrue())

					// save the previous devices and folders and update
					previousDevices := syncthing.Config.Devices
					previousFolders := syncthing.Config.Folders
					syncthing.UpdateDevices(peerListSubset)

					// expect the new devices to have shrunk overall
					Expect(len(syncthing.Config.Devices)).To(BeNumerically("<", len(previousDevices)))
					Expect(len(syncthing.Config.Folders[0].Devices)).To(BeNumerically("<", len(previousFolders[0].Devices)))

					// new device lengths should equal what we passed in
					Expect(len(syncthing.Config.Devices)).To(Equal(len(peerListSubset)))
					Expect(len(syncthing.Config.Folders[0].Devices)).To(Equal(len(peerListSubset)))

					// ensure that the devices in the Syncthing config are only those specified in the subset
					found := 0
					for _, device := range syncthing.Config.Devices {
						foundOne := false
						// peer must exist in the devices
						for _, peer := range peerListSubset {
							if device.DeviceID == peer.ID {
								found++
								foundOne = true
								break
							}
						}
						Expect(foundOne).To(BeTrue())
					}
					Expect(found).To(Equal(len(syncthing.Config.Devices)))

					// expect the new devices to be shared with the folder
					found = 0
					for _, device := range syncthing.Config.Folders[0].Devices {
						foundOne := false
						for _, peer := range peerListSubset {
							if device.DeviceID == peer.ID {
								found++
								foundOne = true
								break
							}
						}
						Expect(foundOne).To(BeTrue())
					}
					Expect(found).To(Equal(len(syncthing.Config.Folders[0].Devices)))
				})
			})
		})

		When("APIConfig is used", func() {
			var apiConfig *APIConfig
			BeforeEach(func() {
				apiConfig = &APIConfig{
					APIURL: "https://not.a.real.url",
				}
			})

			It("cannot create headers without an APIKey", func() {
				_, err := apiConfig.Headers()
				Expect(err).To(HaveOccurred())
			})

			When("an APIKey is provided", func() {
				BeforeEach(func() {
					apiConfig.APIKey = "BOSCO"
				})
				It("creates headers", func() {
					headers, err := apiConfig.Headers()
					Expect(err).ToNot(HaveOccurred())
					Expect(headers).To(HaveKeyWithValue("X-API-Key", "BOSCO"))
				})
			})

			When("no TLSClient exists", func() {
				BeforeEach(func() {
					apiConfig.Client = nil
				})
				It("creates a new client", func() {
					tlsclient := apiConfig.BuildOrUseExistingTLSClient()
					Expect(tlsclient).ToNot(BeNil())
				})
			})

			When("TLSClient exists", func() {
				BeforeEach(func() {
					apiConfig.Client = &http.Client{
						Timeout: time.Hour * 273,
					}
				})
				It("uses the existing client", func() {
					httpClient := apiConfig.BuildOrUseExistingTLSClient()
					Expect(httpClient).ToNot(BeNil())
					Expect(httpClient.Timeout).To(Equal(time.Hour * 273))
				})
			})
		})
	})
	Context("TLS Certificates are generated", func() {
		It("generates them without fault", func() {
			var apiAddress string = "my.real.api.address"
			certPEM, certKeyPEM, err := GenerateTLSCertificatesForSyncthing(apiAddress)
			Expect(err).ToNot(HaveOccurred())
			Expect(certPEM.Bytes()).ToNot(BeEmpty())
			Expect(certKeyPEM.Bytes()).ToNot(BeEmpty())

			// check that the cert can be used
			cert, err := tls.X509KeyPair(certPEM.Bytes(), certKeyPEM.Bytes())
			Expect(err).ToNot(HaveOccurred())
			Expect(cert).ToNot(BeNil())
			Expect(cert.Leaf).To(BeNil())
			Expect(cert.Certificate[0]).ToNot(BeNil())
			Expect(cert.PrivateKey).ToNot(BeNil())
		})
	})
})
