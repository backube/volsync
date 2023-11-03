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
	"net/http/httptest"
	"os"
	"strconv"
	"strings"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	cMover "github.com/backube/volsync/controllers/mover"
	"github.com/backube/volsync/controllers/mover/syncthing/api"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/protocol"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/events"
	"k8s.io/utils/ptr"
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
			for _, v := range cMover.Catalog {
				if _, ok := v.(*Builder); ok {
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
		mover, err := commonBuilderForTestSuite.FromSource(k8sClient, logger, &events.FakeRecorder{}, rs,
			true /* privileged */)
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

		It("is nil", func() {
			// make sure that syncthing never works with ReplicationDestination
			rd = &volsyncv1alpha1.ReplicationDestination{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "syncthing-rd",
					Namespace: ns.Name,
				},
				Spec: volsyncv1alpha1.ReplicationDestinationSpec{},
			}

			// get a builder from the xRD to ensure that this errors
			m, e := commonBuilderForTestSuite.FromDestination(k8sClient, logger, &events.FakeRecorder{}, rd,
				true /* privileged */)
			Expect(e).To(BeNil())
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
		Expect(k8sClient.Create(ctx, srcPVC)).To(Succeed())

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
		var apiConfig *api.APIConfig
		BeforeEach(func() {
			apiConfig = &api.APIConfig{}
		})
		JustBeforeEach(func() {
			// Set status for controller
			rs.Status = &volsyncv1alpha1.ReplicationSourceStatus{}

			// create a syncthing mover
			m, err := commonBuilderForTestSuite.FromSource(k8sClient, logger, &events.FakeRecorder{}, rs,
				true /* privileged */)
			Expect(err).NotTo(HaveOccurred())
			Expect(m).NotTo(BeNil())

			// cast as a mover
			mover, _ = m.(*Mover)
			Expect(mover).NotTo(BeNil())
			Expect(mover.owner).NotTo(BeNil())

			// set the apiConfig
			mover.apiConfig = *apiConfig
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
			var configPVC *corev1.PersistentVolumeClaim
			var apiSecret *corev1.Secret
			var sa *corev1.ServiceAccount

			// nolint:dupl
			JustBeforeEach(func() {
				var err error

				// create necessary elements
				configPVC = &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      cMover.VolSyncPrefix + mover.owner.GetName(),
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
						Name:      cMover.VolSyncPrefix + mover.owner.GetName(),
						Namespace: ns.Name,
					},
					Data: map[string][]byte{
						apiKeyDataKey:    []byte("my-secret-apikey-do-not-steal"),
						usernameDataKey:  []byte("gcostanza"),
						passwordDataKey:  []byte("bosco"),
						httpsKeyDataKey:  []byte(`-----BEGIN RSA PRIVATE KEY-----123-----END RSA PRIVATE KEY-----`),
						httpsCertDataKey: []byte(`-----BEGIN CERTIFICATE-----123-----END CERTIFICATE-----`),
					},
				}
				Expect(k8sClient.Create(ctx, apiSecret)).To(Succeed())

				// ensure the SA exists
				sa, err = mover.saHandler.Reconcile(ctx, logger)
				Expect(sa).NotTo(BeNil())
				Expect(err).NotTo(HaveOccurred())

				// ensure the SA exists in-cluster
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sa.Name, Namespace: ns.Name}, sa)).To(Succeed())
			})

			When("data serviceType is ClusterIP", func() {
				BeforeEach(func() {
					// set the service type
					svcType = corev1.ServiceTypeClusterIP
					rs.Spec.Syncthing.ServiceType = &svcType
				})

				It("ClusterIP is created", func() {
					// get a deployment
					deployment, err := mover.ensureDeployment(ctx, srcPVC, configPVC, sa, apiSecret)
					Expect(err).NotTo(HaveOccurred())
					Expect(deployment).NotTo(BeNil())

					// make sure the service is created
					svc, err := mover.ensureDataService(ctx, deployment)
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
					Expect(addr).To(Equal("tcp://" + staticIP + ":" + strconv.Itoa(dataPort)))
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
					// get a deployment
					deployment, err := mover.ensureDeployment(ctx, srcPVC, configPVC, sa, apiSecret)
					Expect(err).NotTo(HaveOccurred())
					Expect(deployment).NotTo(BeNil())

					// make sure the service is created
					svc, err := mover.ensureDataService(ctx, deployment)
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
					Expect(address).To(Equal("tcp://" + staticIP + ":" + strconv.Itoa(dataPort)))

					// ensure address works when hostname is provided
					svc.Status.LoadBalancer.Ingress[0].Hostname = staticHostName
					address, e = mover.GetDataServiceAddress(svc)
					Expect(e).NotTo(HaveOccurred())
					Expect(address).To(Equal("tcp://" + staticHostName + ":" + strconv.Itoa(dataPort)))
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
			var dataPVC *corev1.PersistentVolumeClaim
			JustBeforeEach(func() {
				// ensure the data PVC before testing the config PVC so the volumehandler is set
				pvc, err := mover.ensureDataPVC(ctx)
				dataPVC = pvc
				Expect(err).NotTo(HaveOccurred())
			})

			When("configPVC already exists", func() {
				JustBeforeEach(func() {
					// create a config PVC
					configPVC = &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      cMover.VolSyncPrefix + mover.owner.GetName() + "-config",
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
					Expect(k8sClient.Get(ctx, types.NamespacedName{
						Name: configPVC.Name, Namespace: configPVC.Namespace}, configPVC)).To(Succeed())
				})
			})

			When("config options are provided", func() {
				accessModes := []corev1.PersistentVolumeAccessMode{
					corev1.ReadOnlyMany,
				}
				configsc := "configsc"
				configCapacity := resource.MustParse("3Gi")
				BeforeEach(func() {
					rs.Spec.Syncthing.ConfigAccessModes = accessModes
					rs.Spec.Syncthing.ConfigStorageClassName = &configsc
					rs.Spec.Syncthing.ConfigCapacity = &configCapacity
				})

				It("sets the options", func() {
					config, err := mover.ensureConfigPVC(ctx, dataPVC)
					Expect(err).NotTo(HaveOccurred())
					Expect(config).NotTo(BeNil())
					Expect(config.Spec.AccessModes).To(Equal(accessModes))
					Expect(config.Spec.StorageClassName).To(Equal(&configsc))
					// dereference to check object equality
					Expect(*config.Spec.Resources.Requests.Storage()).To(Equal(configCapacity))
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
							Name:      cMover.VolSyncPrefix + mover.owner.GetName(),
							Namespace: ns.Name,
						},
						Data: map[string][]byte{
							apiKeyDataKey:   []byte("my-secret-apikey-do-not-steal"),
							usernameDataKey: []byte("gcostanza"),
							passwordDataKey: []byte("bosco"),
						},
					}
					Expect(k8sClient.Create(ctx, apiKeys)).To(Succeed())
				})

				It("VolSync retrieves secret", func() {
					// retrieve the secret
					returnedSecret, err := mover.ensureSecretAPIKey(ctx)
					Expect(err).NotTo(HaveOccurred())
					Expect(returnedSecret).NotTo(BeNil())
					Expect(returnedSecret.Name).To(Equal(apiKeys.Name))

					// ensure the data keys exist
					Expect(returnedSecret.Data[apiKeyDataKey]).NotTo(BeNil())
					Expect(returnedSecret.Data[usernameDataKey]).NotTo(BeNil())
					Expect(returnedSecret.Data[passwordDataKey]).NotTo(BeNil())
				})
			})

			When("VolSync creates the secret", func() {
				It("VolSync creates the secret", func() {
					// create the secret
					returnedSecret, err := mover.ensureSecretAPIKey(ctx)
					Expect(err).NotTo(HaveOccurred())
					Expect(returnedSecret).NotTo(BeNil())

					// ensure the secret is created
					Expect(k8sClient.Get(ctx, types.NamespacedName{
						Name:      returnedSecret.Name,
						Namespace: returnedSecret.Namespace,
					}, returnedSecret)).Should(Succeed())

					// ensure that all of the required keys exist
					requiredKeys := []string{apiKeyDataKey, usernameDataKey, passwordDataKey, httpsCertDataKey, httpsKeyDataKey}
					for _, key := range requiredKeys {
						Expect(returnedSecret.Data[key]).NotTo(BeNil())
						Expect(returnedSecret.Data[key]).NotTo(BeEmpty())
					}
				})
			})
		})

		Context("Syncthing API is being used properly", func() {
			var (
				myID, _    = protocol.DeviceIDFromString("ZNWFSWE-RWRV2BD-45BLMCV-LTDE2UR-4LJDW6J-R5BPWEB-TXD27XJ-IZF5RA4")
				device1, _ = protocol.DeviceIDFromString("AIR6LPZ-7K4PTTV-UXQSMUU-CPQ5YWH-OEDFIIQ-JUG777G-2YQXXR5-YD6AWQR")
				device2, _ = protocol.DeviceIDFromString("GYRZZQB-IRNPV4Z-T7TC52W-EQYJ3TT-FDQW6MW-DFLMU42-SSSU6EM-FBK2VAY")
				device3, _ = protocol.DeviceIDFromString("VNPQDOJ-3V7DEWN-QBCTXF2-LSVNMHL-XTGL4GX-NCGQEXQ-THHBVWR-HVVMEQR")
			)

			When("Syncthing server does not exist", func() {

				JustBeforeEach(func() {
					mover.apiConfig = api.APIConfig{
						APIURL: "https://my-fake-api-address-123",
						APIKey: "not-a-real-api-key",
					}
					mover.syncthingConnection = api.NewConnection(mover.apiConfig, logger)
				})

				It("Doesn't synchronize", func() {
					res, err := mover.Synchronize(ctx)
					Expect(err).ToNot(BeNil())
					Expect(res).To(Equal(cMover.InProgress()))

					syncthing, err := mover.syncthingConnection.Fetch()
					Expect(err).ToNot(BeNil())
					Expect(syncthing).To(BeNil())

					apiKeys := &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      cMover.VolSyncPrefix + mover.owner.GetName(),
							Namespace: ns.Name,
						},
						Data: map[string][]byte{
							apiKeyDataKey:   []byte("my-secret-apikey-do-not-steal"),
							usernameDataKey: []byte("gcostanza"),
							passwordDataKey: []byte("bosco"),
						},
					}

					syncthing = &api.Syncthing{}
					err = mover.ensureIsConfigured(apiKeys, syncthing)
					Expect(err).ToNot(BeNil())

					service := &corev1.Service{
						ObjectMeta: metav1.ObjectMeta{
							Name:      mover.getAPIServiceName(),
							Namespace: mover.owner.GetNamespace(),
						},
					}
					err = mover.ensureStatusIsUpdated(service, syncthing)
					Expect(err).ToNot(BeNil())
				})
			})

			When("Syncthing server exists", func() {
				var ts *httptest.Server
				var syncthingState *api.Syncthing
				var apiKeys *corev1.Secret
				var apiKey = "my-secret-apikey-do-not-steal"

				BeforeEach(func() {
					// initialize the config variables here
					syncthingState = &api.Syncthing{}
				})

				JustBeforeEach(func() {
					// set status to 10
					syncthingState.Configuration.Version = 10
					syncthingState.SystemStatus.MyID = myID.GoString()
					syncthingState.SystemConnections.Total = api.TotalStats{At: "test"}

					ts = api.CreateSyncthingTestServer(syncthingState, apiKey)

					// configure connection
					mover.apiConfig.APIURL = ts.URL
					mover.apiConfig.Client = ts.Client()
					mover.apiConfig.APIKey = apiKey
					mover.syncthingConnection = api.NewConnection(mover.apiConfig, logger)

					// create apikeys secret here so we can use it in the tests
					apiKeys = &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      cMover.VolSyncPrefix + mover.owner.GetName(),
							Namespace: ns.Name,
						},
						Data: map[string][]byte{
							apiKeyDataKey:   []byte(mover.apiConfig.APIKey),
							usernameDataKey: []byte("gcostanza"),
							passwordDataKey: []byte("bosco"),
						},
					}
				})

				JustAfterEach(func() {
					ts.Close()
				})

				It("Fetches the Latest Info", func() {
					syncthing, err := mover.syncthingConnection.Fetch()
					Expect(err).To(BeNil())
					Expect(syncthing.Configuration.Version).To(Equal(10))
					Expect(syncthing.SystemStatus.MyID).To(Equal(myID.GoString()))
					Expect(syncthing.SystemConnections.Total.At).To(Equal("test"))
				})

				It("Updates the Syncthing Config", func() {
					syncthing := &api.Syncthing{
						Configuration: config.Configuration{
							Version: 9,
						},
					}
					err := mover.syncthingConnection.PublishConfig(syncthing.Configuration)
					Expect(err).To(BeNil())
					Expect(syncthingState.Configuration.Version).To(Equal(9))
				})

				It("Ensures it's configured", func() {
					// setup test variables
					mover.peerList = []volsyncv1alpha1.SyncthingPeer{
						{
							Address: "tcp://127.0.0.1:22000",
							ID:      device1.GoString(),
						},
						{
							Address: "tcp://127.0.0.2:22000",
							ID:      device2.GoString(),
						},
					}
					// pull syncthing state from server
					syncthing, err := mover.syncthingConnection.Fetch()
					Expect(err).To(BeNil())

					// configure syncthing server w/ local state
					err = mover.ensureIsConfigured(apiKeys, syncthing)
					Expect(err).To(BeNil())

					// make sure that our peers can be found on the server
					for i, peer := range mover.peerList {
						devID, err := protocol.DeviceIDFromString(peer.ID)
						Expect(err).To(BeNil())
						expected := config.DeviceConfiguration{
							DeviceID:   devID,
							Addresses:  []string{peer.Address},
							Introducer: peer.Introducer,
						}
						Expect(syncthingState.Configuration.Devices[i]).To(Equal(expected))
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

					// setup some dummy peers
					mover.peerList = []volsyncv1alpha1.SyncthingPeer{
						{
							Address: "tcp://127.0.0.1:22000",
							ID:      device1.GoString(),
						},
						{
							Address: "tcp://127.0.0.2:22000",
							ID:      device2.GoString(),
						},
					}

					// update the mover's status with info from the Syncthing server
					syncthing, err := mover.syncthingConnection.Fetch()
					Expect(err).To(BeNil())
					err = mover.ensureStatusIsUpdated(service, syncthing)
					Expect(err).To(BeNil())

					// ensure that volsync is recording whether or not we are connected
					for i, peer := range mover.status.Peers {
						p := volsyncv1alpha1.SyncthingPeer{
							Address: peer.Address,
							ID:      peer.ID,
						}
						Expect(mover.peerList[i]).To(Equal(p))
						// we never connected explicitly
						Expect(peer.Connected).To(BeFalse())
					}
				})

				When("Syncthing has active connections", func() {
					var device3Config = config.DeviceConfiguration{
						DeviceID:     device3,
						Addresses:    []string{"tcp://127.0.0.1:22000"},
						Name:         "bitcoin-chain-snapshot",
						IntroducedBy: device2,
					}

					JustBeforeEach(func() {
						// add some connections here
						syncthingState.SystemConnections.Connections = map[string]api.ConnectionStats{
							myID.GoString(): {
								Connected: false,
								Address:   "",
							},
							device3.GoString(): {
								Connected: true,
								Address:   device3Config.Addresses[0],
							},
						}

						// ensure that another-one is in the global config
						syncthingState.Configuration.SetDevices([]config.DeviceConfiguration{
							device3Config,
							{
								DeviceID:  myID,
								Addresses: []string{""},
							},
						},
						)
					})

					It("adds them to VolSync status", func() {
						// create a data service that Syncthing will use in status updating
						fakeDataSVC := corev1.Service{
							ObjectMeta: metav1.ObjectMeta{
								Name:      cMover.VolSyncPrefix + mover.owner.GetName() + "-data",
								Namespace: ns.Namespace,
							},
							Spec: corev1.ServiceSpec{
								ClusterIP: "1.2.3.4",
								Type:      corev1.ServiceTypeClusterIP,
							},
						}

						// expect status to be updated
						syncthing, err := mover.syncthingConnection.Fetch()
						Expect(err).To(BeNil())
						Expect(syncthing).NotTo(BeNil())
						err = mover.ensureStatusIsUpdated(&fakeDataSVC, syncthing)
						Expect(err).To(BeNil())

						// check that the status contains the new connection
						Expect(mover.status.Peers).To(HaveLen(1))
						peer := mover.status.Peers[0]

						// ensure that volsync properly set these fields
						// ID should be the other peer's; not the local one
						Expect(peer.ID).To(Equal(device3.GoString()))
						Expect(peer.Address).To(Equal(device3Config.Addresses[0]))
						Expect(peer.Connected).To(BeTrue())
						Expect(peer.IntroducedBy).To(Equal(device3Config.IntroducedBy.GoString()))
						Expect(peer.Name).To(Equal(device3Config.Name))
					})
				})

				Context("VolSync is improperly configuring Syncthing", func() {
					JustBeforeEach(func() {
					})

					When("VolSync adds its own Syncthing instance to the mover's peerList", func() {
						It("errors", func() {
							// set the peerlist to itself
							mover.peerList = []volsyncv1alpha1.SyncthingPeer{
								{
									ID:      myID.GoString(),
									Address: "tcp://127.0.0.1:22000",
								},
							}
							syncthing, err := mover.syncthingConnection.Fetch()
							Expect(err).NotTo(HaveOccurred())
							Expect(syncthing).NotTo(BeNil())
							err = mover.ensureIsConfigured(apiKeys, syncthing)
							Expect(err).To(HaveOccurred())
						})
					})
				})
			})
		})

		Context("service account is created", func() {
			It("creates a new one", func() {
				// create the SA
				sa, err := mover.saHandler.Reconcile(ctx, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(sa).NotTo(BeNil())

				// make sure that the SA exists in-cluster
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sa.Name, Namespace: ns.Name}, sa)).To(Succeed())
			})
		})

		Context("A user supplied moverServiceAccount is set in the spec", func() {
			userSuppliedMoverSvcAccount := "cust-svc-acct"
			BeforeEach(func() {
				// Update rsSpec to set our own svc account
				rs.Spec.Syncthing.MoverServiceAccount = &userSuppliedMoverSvcAccount
			})

			When("The mover service account does not exist", func() {
				It("The saHandler should fail to reconcile", func() {
					sa, err := mover.saHandler.Reconcile(ctx, logger)
					Expect(sa).To(BeNil())
					Expect(err).To(HaveOccurred())
					Expect(err).To(HaveOccurred())
				})
			})

			When("The mover service account exists", func() {
				BeforeEach(func() {
					// Create the svc account
					userSvcAccount := &corev1.ServiceAccount{
						ObjectMeta: metav1.ObjectMeta{
							Name:      userSuppliedMoverSvcAccount,
							Namespace: ns.Name,
						},
					}
					Expect(k8sClient.Create(ctx, userSvcAccount)).To(Succeed())
				})
				It("Should use the supplied service account", func() {
					sa, err := mover.saHandler.Reconcile(ctx, logger)
					Expect(err).ToNot(HaveOccurred())
					Expect(sa.GetName()).To(Equal(userSuppliedMoverSvcAccount))
				})
			})
		})

		Context("Syncthing is being properly used", func() {
			var configPVC *corev1.PersistentVolumeClaim
			var apiSecret *corev1.Secret
			var sa *corev1.ServiceAccount
			var deployment *appsv1.Deployment
			var apiService *corev1.Service
			var apiKey string = "test"

			// nolint:dupl
			JustBeforeEach(func() {
				var err error

				// create necessary elements
				configPVC = &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      cMover.VolSyncPrefix + mover.owner.GetName(),
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
						Name:      cMover.VolSyncPrefix + mover.owner.GetName(),
						Namespace: ns.Name,
					},
					Data: map[string][]byte{
						apiKeyDataKey:    []byte(apiKey),
						usernameDataKey:  []byte("gcostanza"),
						passwordDataKey:  []byte("bosco"),
						httpsKeyDataKey:  []byte(`-----BEGIN RSA PRIVATE KEY-----123-----END RSA PRIVATE KEY-----`),
						httpsCertDataKey: []byte(`-----BEGIN CERTIFICATE-----123-----END CERTIFICATE-----`),
					},
				}
				Expect(k8sClient.Create(ctx, apiSecret)).To(Succeed())

				// ensure the SA exists
				sa, err = mover.saHandler.Reconcile(ctx, logger)
				Expect(sa).NotTo(BeNil())
				Expect(err).NotTo(HaveOccurred())

				// ensure the SA exists in-cluster
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sa.Name, Namespace: ns.Name}, sa)).To(Succeed())
			})

			When("the API exists", func() {
				var ts *httptest.Server
				var serverState *api.Syncthing
				var dataService *corev1.Service
				var (
					myID    = "ZNWFSWE-RWRV2BD-45BLMCV-LTDE2UR-4LJDW6J-R5BPWEB-TXD27XJ-IZF5RA4"
					device1 = "AIR6LPZ-7K4PTTV-UXQSMUU-CPQ5YWH-OEDFIIQ-JUG777G-2YQXXR5-YD6AWQR"
				)

				BeforeEach(func() {
					serverState = &api.Syncthing{}
				})

				// create the API server
				JustBeforeEach(func() {
					// configure the Syncthing server config
					serverState.Configuration.Version = 10
					serverState.SystemStatus.MyID = myID
					serverState.SystemConnections.Total = api.TotalStats{At: "test"}

					// configure the test TLS server
					ts = api.CreateSyncthingTestServer(serverState, apiKey)

					// configure the API
					apiConfig := api.APIConfig{}
					apiConfig.APIURL = ts.URL
					apiConfig.Client = ts.Client()
					mover.apiConfig = apiConfig

					// create the data service
					dataService = &corev1.Service{
						ObjectMeta: metav1.ObjectMeta{
							Name:      cMover.VolSyncPrefix + mover.owner.GetName() + "-data",
							Namespace: ns.Name,
						},
						Spec: corev1.ServiceSpec{
							Ports: []corev1.ServicePort{
								{
									Name:       dataPortName,
									Protocol:   corev1.ProtocolTCP,
									Port:       dataPort,
									TargetPort: intstr.FromInt(dataPort),
								},
							},
							Type: corev1.ServiceTypeClusterIP,
						},
					}

					// launch the data service in our cluster
					Expect(k8sClient.Create(ctx, dataService)).To(Succeed())
				})

				// close down the server after testing
				JustAfterEach(func() {
					ts.Close()
				})

				It("successfully completes a Synchronize", func() {
					result, err := mover.Synchronize(ctx)

					// expect no error to have occurred
					Expect(err).NotTo(HaveOccurred())
					// synchronization is eternal
					Expect(result.Completed).To(BeFalse())
				})

				When("peer is added", func() {
					var peer *volsyncv1alpha1.SyncthingPeer
					JustBeforeEach(func() {
						peer = &volsyncv1alpha1.SyncthingPeer{
							Address:    "tcp://1.2.3.4:5678",
							ID:         device1,
							Introducer: false,
						}
						mover.peerList = []volsyncv1alpha1.SyncthingPeer{*peer}
					})

					It("doesn't add duplicates", func() {
						result, err := mover.Synchronize(ctx)

						// expect no error to have occurred
						Expect(err).NotTo(HaveOccurred())
						Expect(result.Completed).To(BeFalse())

						// peer should have been listed in status
						found := 0
						for _, p := range mover.status.Peers {
							if p.ID == peer.ID {
								found++
							}
						}
						Expect(found).To(Equal(1))

						// make sure no duplicates of peer exist in the syncthing state
						Expect(len(serverState.Configuration.Devices)).To(Equal(1))
						Expect(len(serverState.SystemConnections.Connections)).To(Equal(1))

						// run again and ensure no duplicates are added
						result, err = mover.Synchronize(ctx)
						Expect(err).NotTo(HaveOccurred())
						Expect(result.Completed).To(BeFalse())
						found = 0
						for _, p := range mover.status.Peers {
							if p.ID == peer.ID {
								found++
							}
						}
						Expect(found).To(Equal(1))
					})
				})
			})

			When("synchronize is called but server isn't running", func() {
				It("errors", func() {
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
				When("No deployment currently exists", func() {
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

						Expect(stContainer.Env).To(HaveLen(5)) // secret env vars + PRIVILEGED_MOVER env var
						for _, env := range stContainer.Env {
							if env.Name == apiKeyEnv {
								Expect(env.ValueFrom.SecretKeyRef.Name).To(Equal(apiSecret.Name))
								Expect(env.ValueFrom.SecretKeyRef.Key).To(Equal(apiKeyDataKey))
							}
						}

						// validate that the deployment exposes API & data ports
						var portNames []string = []string{apiPortName, dataPortName}

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
						necessaryVolumeMounts := []string{
							configVolumeName,
							dataVolumeName,
							certVolumeName,
						}
						found := 0
						for _, volume := range deployment.Spec.Template.Spec.Volumes {
							for _, mount := range necessaryVolumeMounts {
								if volume.Name == mount {
									found++
									break
								}
							}
						}
						Expect(found).To(Equal(len(necessaryVolumeMounts)))

						checked := 0
						for _, volume := range deployment.Spec.Template.Spec.Volumes {
							if volume.Name == configVolumeName {
								Expect(volume.PersistentVolumeClaim.ClaimName).To(Equal(configPVC.Name))
								checked++
							} else if volume.Name == dataVolumeName {
								Expect(volume.PersistentVolumeClaim.ClaimName).To(Equal(srcPVC.Name))
								checked++
							} else if volume.Name == certVolumeName {
								// check that the TLS certificates are properly loaded
								Expect(volume.Secret.SecretName).To(Equal(apiSecret.Name))

								// make sure the tls certificates are mounted as secret keys
								httpsKeysChecked := 0
								httpsItems := []string{httpsKeyDataKey, httpsCertDataKey}
								for _, item := range volume.Secret.Items {
									for _, httpItem := range httpsItems {
										if item.Key == httpItem {
											httpsKeysChecked++
										}
									}
								}
								Expect(httpsKeysChecked).To(Equal(len(httpsItems)))
								checked++
							}
						}
						// make sure that all volumes are accounted for
						Expect(checked).To(Equal(len(deployment.Spec.Template.Spec.Volumes)))

						// make sure that both deployment's specified volumes are being mounted by the container
						for _, mount := range stContainer.VolumeMounts {
							if mount.Name == configVolumeName {
								Expect(mount.MountPath).To(Equal(configDirMountPath))
							} else if mount.Name == dataVolumeName {
								Expect(mount.MountPath).To(Equal(dataDirMountPath))
							} else if mount.Name == certVolumeName {
								Expect(mount.MountPath).To(Equal(certDirMountPath))
							}
						}

						// No proxy env vars should be set by default
						envVars := deployment.Spec.Template.Spec.Containers[0].Env
						for _, envVar := range envVars {
							Expect(strings.ToLower(envVar.Name)).NotTo(ContainSubstring("proxy"))
						}
					})

					Context("Cluster wide proxy settings", func() {
						httpProxy := "http://myproxy:1234"
						httpsProxy := "https://10.10.10.1"
						noProxy := "*.abc.com, 10.11.11.200"
						BeforeEach(func() {
							os.Setenv("HTTP_PROXY", httpProxy)
							os.Setenv("HTTPS_PROXY", httpsProxy)
							os.Setenv("NO_PROXY", noProxy)
						})
						AfterEach(func() {
							os.Unsetenv("HTTP_PROXY")
							os.Unsetenv("HTTPS_PROXY")
							os.Unsetenv("NO_PROXY")
						})
						It("Should inherit cluster wide proxy env vars from the volsync controller", func() {
							deployment, err := mover.ensureDeployment(ctx, srcPVC, configPVC, sa, apiSecret)
							Expect(err).NotTo(HaveOccurred())
							Expect(deployment).NotTo(BeNil())

							envVars := deployment.Spec.Template.Spec.Containers[0].Env
							Expect(envVars).To(ContainElement(corev1.EnvVar{Name: "HTTPS_PROXY", Value: httpsProxy}))
							Expect(envVars).To(ContainElement(corev1.EnvVar{Name: "https_proxy", Value: httpsProxy}))
							Expect(envVars).To(ContainElement(corev1.EnvVar{Name: "HTTP_PROXY", Value: httpProxy}))
							Expect(envVars).To(ContainElement(corev1.EnvVar{Name: "http_proxy", Value: httpProxy}))
							Expect(envVars).To(ContainElement(corev1.EnvVar{Name: "NO_PROXY", Value: noProxy}))
							Expect(envVars).To(ContainElement(corev1.EnvVar{Name: "no_proxy", Value: noProxy}))
						})
					})
					Context("Privileged vs unprivileged mover", func() {
						It("Should not have a PodSecurityContext by default", func() {
							deployment, err := mover.ensureDeployment(ctx, srcPVC, configPVC, sa, apiSecret)
							Expect(err).NotTo(HaveOccurred())
							Expect(deployment).NotTo(BeNil())

							podSpec := deployment.Spec.Template.Spec
							Expect(podSpec.SecurityContext).NotTo(BeNil())
							Expect(podSpec.SecurityContext.RunAsUser).To(BeNil())
							Expect(podSpec.SecurityContext.FSGroup).To(BeNil())

						})

						When("A moverSecurityContext is provided", func() {
							BeforeEach(func() {
								rs.Spec.Syncthing.MoverSecurityContext = &corev1.PodSecurityContext{
									RunAsUser: ptr.To[int64](7),
									FSGroup:   ptr.To[int64](8),
								}
							})
							It("Should appear in the mover Job", func() {
								deployment, err := mover.ensureDeployment(ctx, srcPVC, configPVC, sa, apiSecret)
								Expect(err).NotTo(HaveOccurred())
								Expect(deployment).NotTo(BeNil())

								psc := deployment.Spec.Template.Spec.SecurityContext
								Expect(psc).NotTo(BeNil())
								Expect(psc.RunAsUser).NotTo(BeNil())
								Expect(*psc.RunAsUser).To(Equal(int64(7)))
								Expect(psc.FSGroup).NotTo(BeNil())
								Expect(*psc.FSGroup).To(Equal(int64(8)))
							})
						})

						When("The NS does not allow privileged movers", func() {
							It("Should run unprivileged", func() {
								mover.privileged = false // Mover created with true above, change for this test

								deployment, err := mover.ensureDeployment(ctx, srcPVC, configPVC, sa, apiSecret)
								Expect(err).NotTo(HaveOccurred())
								Expect(deployment).NotTo(BeNil())

								stContainer := deployment.Spec.Template.Spec.Containers[0]
								Expect(*stContainer.SecurityContext.AllowPrivilegeEscalation).To(Equal(false))
								Expect(*stContainer.SecurityContext.Privileged).To(Equal(false))
								Expect(*stContainer.SecurityContext.ReadOnlyRootFilesystem).To(Equal(true))

								Expect(stContainer.SecurityContext.Capabilities).To(Equal(&corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								}))
								Expect(stContainer.SecurityContext.RunAsUser).To(BeNil())

								foundPrivilegedMoverEnvVar := false
								for _, envVar := range stContainer.Env {
									if envVar.Name == "PRIVILEGED_MOVER" {
										foundPrivilegedMoverEnvVar = true
										Expect(envVar.Value).To(Equal("0"))
										break
									}
								}
								Expect(foundPrivilegedMoverEnvVar).To(BeTrue())
							})
						})

						When("The NS allows privileged movers", func() { // Already the case in this block
							It("Should start a privileged mover", func() {
								deployment, err := mover.ensureDeployment(ctx, srcPVC, configPVC, sa, apiSecret)
								Expect(err).NotTo(HaveOccurred())
								Expect(deployment).NotTo(BeNil())

								stContainer := deployment.Spec.Template.Spec.Containers[0]
								Expect(*stContainer.SecurityContext.AllowPrivilegeEscalation).To(Equal(false))
								Expect(*stContainer.SecurityContext.Privileged).To(Equal(false))
								Expect(*stContainer.SecurityContext.ReadOnlyRootFilesystem).To(Equal(true))

								Expect(len(stContainer.SecurityContext.Capabilities.Add)).To(BeNumerically(">", 0))
								Expect(stContainer.SecurityContext.RunAsUser).NotTo(BeNil())
								Expect(*stContainer.SecurityContext.RunAsUser).To(Equal(int64(0)))

								foundPrivilegedMoverEnvVar := false
								for _, envVar := range stContainer.Env {
									if envVar.Name == "PRIVILEGED_MOVER" {
										foundPrivilegedMoverEnvVar = true
										Expect(envVar.Value).To(Equal("1"))
										break
									}
								}
								Expect(foundPrivilegedMoverEnvVar).To(BeTrue())
							})
						})
					})
				})

				When("Deployment already exists", func() {
					JustBeforeEach(func() {
						// need to declare this err so scoped deployment can be used in test
						var err error

						// create a deployment
						deployment, err = mover.ensureDeployment(ctx, srcPVC, configPVC, sa, apiSecret)
						Expect(err).NotTo(HaveOccurred())
						Expect(deployment).NotTo(BeNil())

						// Check the deployment exists
						Expect(k8sClient.Get(ctx, types.NamespacedName{
							Name: deployment.Name, Namespace: ns.Name}, deployment)).To(Succeed())
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
							Expect(k8sClient.Get(ctx, types.NamespacedName{
								Name: apiService.Name, Namespace: ns.Name}, apiService)).To(Succeed())
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
							Expect(k8sClient.Get(ctx, types.NamespacedName{
								Name: apiService.Name, Namespace: ns.Name}, apiService)).To(Succeed())

							// ensure that the service's ports match the deployments
							Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
							syncthingContainer := deployment.Spec.Template.Spec.Containers[0]

							// ensure that the deployment's API port matches the service's
							Expect(apiService.Spec.Ports).To(HaveLen(1))

							// find the API port
							var apiPort corev1.ContainerPort
							for _, port := range syncthingContainer.Ports {
								if port.Name == apiPortName {
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
		m, err := commonBuilderForTestSuite.FromSource(k8sClient, logger, &events.FakeRecorder{}, rs,
			true /* privileged */)
		Expect(err).ToNot(HaveOccurred())
		Expect(m).ToNot(BeNil())
	})
})

// These tests describe the behavior of the Syncthing struct and ensures that its methods
// are working as expected.
var _ = Describe("Syncthing utils", func() {
	var (
		myID, _    = protocol.DeviceIDFromString("ZNWFSWE-RWRV2BD-45BLMCV-LTDE2UR-4LJDW6J-R5BPWEB-TXD27XJ-IZF5RA4")
		device1, _ = protocol.DeviceIDFromString("AIR6LPZ-7K4PTTV-UXQSMUU-CPQ5YWH-OEDFIIQ-JUG777G-2YQXXR5-YD6AWQR")
		device2, _ = protocol.DeviceIDFromString("GYRZZQB-IRNPV4Z-T7TC52W-EQYJ3TT-FDQW6MW-DFLMU42-SSSU6EM-FBK2VAY")
		device3, _ = protocol.DeviceIDFromString("VNPQDOJ-3V7DEWN-QBCTXF2-LSVNMHL-XTGL4GX-NCGQEXQ-THHBVWR-HVVMEQR")
		device4, _ = protocol.DeviceIDFromString("E3TWU3G-UGFHTJE-SJLCDYH-KGQR3R6-7QMOM43-FOC3UFT-H4H54DC-GMK5RAO")
	)

	Context("Syncthing object is used", func() {
		var syncthing api.Syncthing

		// logger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))

		// Configure each Syncthing object beforehand
		BeforeEach(func() {

			// initialize these pointer fields
			syncthing = api.Syncthing{}
			syncthing.SystemStatus.MyID = myID.GoString()
		})

		When("devices are called to update", func() {
			BeforeEach(func() {
				// create a folder
				syncthing.Configuration.Folders = append(syncthing.Configuration.Folders, config.FolderConfiguration{
					ID:    string(sha256.New().Sum([]byte("festivus-files-1986"))),
					Label: "festivus-files",
				})
			})

			It("updates them based on the provided peerList", func() {
				// create a peer list
				peerList := []volsyncv1alpha1.SyncthingPeer{
					{
						ID:      device1.GoString(),
						Address: "tcp://127.0.0.1:22000",
					},
					{
						ID:      device2.GoString(),
						Address: "tcp://192.168.1.1:22000",
					},
				}

				// ensure that the devices are updated
				err := updateSyncthingDevices(peerList, &syncthing)
				Expect(err).NotTo(HaveOccurred())
				Expect(syncthing.Configuration.Devices).To(HaveLen(2))

				// ensure that we can discover all of the devices on the local object
				discovered := 0
				for _, device := range syncthing.Configuration.Devices {
					if device.DeviceID.GoString() == device1.GoString() {
						discovered++
					}
					if device.DeviceID.GoString() == device2.GoString() {
						discovered++
					}
				}

				// we should have found all of the devices
				Expect(discovered).To(Equal(len(syncthing.Configuration.Devices)))

				// folders should have been shared with the new peers
				for _, folder := range syncthing.Configuration.Folders {
					Expect(len(folder.Devices)).To(Equal(len(peerList)))
				}

				// pass an empty peer list to ensure that the devices are removed
				err = updateSyncthingDevices([]volsyncv1alpha1.SyncthingPeer{}, &syncthing)
				Expect(err).NotTo(HaveOccurred())
				Expect(syncthing.Configuration.Devices).To(HaveLen(0))
				for _, folder := range syncthing.Configuration.Folders {
					Expect(folder.Devices).To(HaveLen(0))
				}
			})

			When("syncthing lists itself within the devices entries", func() {
				BeforeEach(func() {
					// make sure that the syncthing is listed in the connections
					syncthing.Configuration.Devices = append(syncthing.Configuration.Devices, config.DeviceConfiguration{
						DeviceID:  myID,
						Name:      "current Syncthing node",
						Addresses: []string{"tcp://0.0.0.0:22000"},
					})
				})

				It("retains the entry when updated against an empty peerlist", func() {
					// pass an empty peer list and make sure that we can still find the self device
					err := updateSyncthingDevices([]volsyncv1alpha1.SyncthingPeer{}, &syncthing)
					Expect(err).NotTo(HaveOccurred())
					Expect(syncthing.Configuration.Devices).To(HaveLen(1))
					Expect(syncthing.Configuration.Devices[0].DeviceID.GoString()).To(Equal(syncthing.SystemStatus.MyID))
				})

				It("only reconfigures when other syncthing devices are provided", func() {
					// pass an empty peer list and make sure that Syncthing doesn't need to be reconfigured
					Expect(syncthingNeedsReconfigure([]volsyncv1alpha1.SyncthingPeer{}, &syncthing)).To(BeFalse())

					// pass a peer list containing self to ensure that Syncthing doesn't need to be reconfigured
					Expect(syncthingNeedsReconfigure([]volsyncv1alpha1.SyncthingPeer{
						{
							ID:      myID.GoString(),
							Address: "tcp://127.0.0.1:22000",
						},
					}, &syncthing)).To(BeFalse())

					// specify a peer
					Expect(syncthingNeedsReconfigure([]volsyncv1alpha1.SyncthingPeer{
						{
							ID:      device1.GoString(),
							Address: "tcp://[::1]:22000",
						},
					}, &syncthing)).To(BeTrue())
				})
			})

			When("syncthing has an empty device list", func() {
				BeforeEach(func() {
					// clear Syncthing's device list and make sure that we can still find the self device
					syncthing.Configuration.Devices = []config.DeviceConfiguration{}
				})

				It("only reconfigures when other syncthing devices are provided", func() {
					// test with an empty list
					peerList := []volsyncv1alpha1.SyncthingPeer{}
					needsReconfigure := syncthingNeedsReconfigure(peerList, &syncthing)
					Expect(needsReconfigure).To(BeFalse())

					// specify ourself as a peer
					peerList = []volsyncv1alpha1.SyncthingPeer{
						{
							ID:      myID.GoString(),
							Address: "",
						},
					}
					needsReconfigure = syncthingNeedsReconfigure(peerList, &syncthing)
					Expect(needsReconfigure).To(BeFalse())

					// specify a peer
					peerList = []volsyncv1alpha1.SyncthingPeer{
						{
							ID:      device1.GoString(),
							Address: "tcp://[::1]:22000",
						},
					}
					needsReconfigure = syncthingNeedsReconfigure(peerList, &syncthing)
					Expect(needsReconfigure).To(BeTrue())
				})
			})

			When("other devices are configured", func() {
				BeforeEach(func() {
					syncthingPeers := []volsyncv1alpha1.SyncthingPeer{
						{
							ID:      device1.GoString(),
							Address: "tcp://[::1]:22000",
						},
						{
							ID:      device2.GoString(),
							Address: "tcp://[::2]:22000",
						},
						{
							ID:      device3.GoString(),
							Address: "tcp://[::3]:22000",
						},
					}
					err := updateSyncthingDevices(syncthingPeers, &syncthing)
					Expect(err).NotTo(HaveOccurred())
				})

				It("doesn't reconfigure when no new devices are passed in", func() {
					// create a peerlist with the same devices as the current config
					peerList := []volsyncv1alpha1.SyncthingPeer{
						{
							ID:      device1.GoString(),
							Address: "tcp://[::1]:22000",
						},
						{
							ID:      device2.GoString(),
							Address: "tcp://[::2]:22000",
						},
						{
							ID:      device3.GoString(),
							Address: "tcp://[::3]:22000",
						},
					}

					// Syncthing should not reconfigure when the peerlist is the same
					needsReconfigure := syncthingNeedsReconfigure(peerList, &syncthing)
					Expect(needsReconfigure).To(BeFalse())

					// make sure that nothing new is updated
					previousDevices := syncthing.Configuration.Devices
					previousFolders := syncthing.Configuration.Folders
					err := updateSyncthingDevices(peerList, &syncthing)
					Expect(err).NotTo(HaveOccurred())

					// make sure that all of the previous devices are the same as the current devices
					found := 0
					for _, device := range syncthing.Configuration.Devices {
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
					Expect(found).To(Equal(len(syncthing.Configuration.Devices)))

					// expect to find all of the current devices in the folder in the previous folder's list
					found = 0
					for _, device := range syncthing.Configuration.Folders[0].Devices {
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
					Expect(found).To(Equal(len(syncthing.Configuration.Folders[0].Devices)))
				})

				It("only needs reconfigure when the list differs but ignores the self syncthing device", func() {
					// test with an empty list
					peerList := []volsyncv1alpha1.SyncthingPeer{}
					needsReconfigure := syncthingNeedsReconfigure(peerList, &syncthing)
					Expect(needsReconfigure).To(BeTrue())

					// Syncthing should view this as erasing all peers
					peerList = []volsyncv1alpha1.SyncthingPeer{
						{
							ID:      syncthing.SystemStatus.MyID,
							Address: "tcp://[::1]:22000",
						},
					}
					Expect(syncthingNeedsReconfigure(peerList, &syncthing)).To(BeTrue())
				})

				It("can reconfigure to a larger list", func() {
					// create a peerlist based on the configured devices in the Syncthing object
					replicaPeerList := []volsyncv1alpha1.SyncthingPeer{}
					for _, device := range syncthing.Configuration.Devices {
						replicaPeerList = append(replicaPeerList, volsyncv1alpha1.SyncthingPeer{
							ID:      device.DeviceID.GoString(),
							Address: device.Addresses[0],
						})
					}
					// specify an additional peer
					replicaPeerList = append(replicaPeerList,
						volsyncv1alpha1.SyncthingPeer{
							ID:      device4.GoString(),
							Address: "tcp://256.256.256.256:22000",
						},
					)
					Expect(syncthingNeedsReconfigure(replicaPeerList, &syncthing)).To(BeTrue())

					// update the Syncthing config with the new peerlist
					err := updateSyncthingDevices(replicaPeerList, &syncthing)
					Expect(err).NotTo(HaveOccurred())

					// expect to find the new peer in the config
					found := false
					for _, device := range syncthing.Configuration.Devices {
						found = device.DeviceID.GoString() == device4.GoString()
						if found {
							break
						}
					}
					Expect(found).To(BeTrue())

					// expect to find the new peer in the folder
					found = false
					for _, device := range syncthing.Configuration.Folders[0].Devices {
						found = device.DeviceID.GoString() == device4.GoString()
						if found {
							break
						}
					}
					Expect(found).To(BeTrue())
				})

				It("reconfigures to a smaller list", func() {
					// create a smaller peerlist with a subset of the devices in the current config
					peerList := []volsyncv1alpha1.SyncthingPeer{
						{
							ID:      device1.GoString(),
							Address: "tcp://[::1]:22000",
						},
						{
							ID:      device2.GoString(),
							Address: "tcp://[::2]:22000",
						},
						{
							ID:      device3.GoString(),
							Address: "tcp://[::3]:22000",
						},
					}
					peerToRemove := peerList[1]

					// only specify a subset of the peers
					peerListSubset := []volsyncv1alpha1.SyncthingPeer{}
					for _, peer := range peerList {
						if peer.ID != peerToRemove.ID {
							peerListSubset = append(peerListSubset, peer)
						}
					}

					// syncthing must reconfigure to exclude the missing peer
					Expect(syncthingNeedsReconfigure(peerListSubset, &syncthing)).To(BeTrue())

					// save the previous devices and folders and update
					previousDevices := syncthing.Configuration.Devices
					previousFolders := syncthing.Configuration.Folders
					err := updateSyncthingDevices(peerListSubset, &syncthing)
					Expect(err).NotTo(HaveOccurred())

					// expect the new devices to have shrunk overall
					Expect(len(syncthing.Configuration.Devices)).To(BeNumerically("<", len(previousDevices)))
					Expect(len(syncthing.Configuration.Folders[0].Devices)).To(BeNumerically("<", len(previousFolders[0].Devices)))

					// new device lengths should equal what we passed in
					Expect(len(syncthing.Configuration.Devices)).To(Equal(len(peerListSubset)))
					Expect(len(syncthing.Configuration.Folders[0].Devices)).To(Equal(len(peerListSubset)))

					// ensure that the devices in the Syncthing config are only those specified in the subset
					found := 0
					for _, device := range syncthing.Configuration.Devices {
						foundOne := false
						// peer must exist in the devices
						for _, peer := range peerListSubset {
							if device.DeviceID.GoString() == peer.ID {
								found++
								foundOne = true
								break
							}
						}
						Expect(foundOne).To(BeTrue())
					}
					Expect(found).To(Equal(len(syncthing.Configuration.Devices)))

					// expect the new devices to be shared with the folder
					found = 0
					for _, device := range syncthing.Configuration.Folders[0].Devices {
						foundOne := false
						for _, peer := range peerListSubset {
							if device.DeviceID.GoString() == peer.ID {
								found++
								foundOne = true
								break
							}
						}
						Expect(foundOne).To(BeTrue())
					}
					Expect(found).To(Equal(len(syncthing.Configuration.Folders[0].Devices)))
				})
			})
		})

	})
	Context("TLS Certificates are generated", func() {
		It("generates them without fault", func() {
			var apiAddress string = "my.real.api.address"
			certPEM, certKeyPEM, err := generateTLSCertificatesForSyncthing(apiAddress)
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
