package api

// Context("Syncthing API is being used properly", func() {
// 	When("Syncthing server does not exist", func() {
// 		It("Doesn't synchronize", func() {
// 			res, err := mover.Synchronize(ctx)
// 			Expect(err).ToNot(BeNil())
// 			Expect(res).To(Equal(cMover.InProgress()))

// 			err = mover.syncthing.FetchSyncthingConfig()
// 			Expect(err).ToNot(BeNil())
// 			Expect(err.Error()).To(ContainSubstring("Get"))

// 			err = mover.syncthing.FetchSyncthingSystemStatus()
// 			Expect(err).ToNot(BeNil())
// 			Expect(err.Error()).To(ContainSubstring("Get"))

// 			err = mover.syncthing.FetchConnectedStatus()
// 			Expect(err).ToNot(BeNil())
// 			Expect(err.Error()).To(ContainSubstring("Get"))

// 			apiKeys := &corev1.Secret{
// 				ObjectMeta: metav1.ObjectMeta{
// 					Name:      "volsync-" + mover.owner.GetName(),
// 					Namespace: ns.Name,
// 				},
// 				Data: map[string][]byte{
// 					"apikey":   []byte("my-secret-apikey-do-not-steal"),
// 					"username": []byte("gcostanza"),
// 					"password": []byte("bosco"),
// 				},
// 			}

// 			err = mover.ensureIsConfigured(apiKeys)
// 			Expect(err).ToNot(BeNil())
// 			Expect(err.Error()).To(ContainSubstring("no such host"))
// 			Expect(err.Error()).To(ContainSubstring("Get"))

// 			service := &corev1.Service{
// 				ObjectMeta: metav1.ObjectMeta{
// 					Name:      mover.getAPIServiceName(),
// 					Namespace: mover.owner.GetNamespace(),
// 				},
// 			}
// 			err = mover.ensureStatusIsUpdated(service)
// 			Expect(err).ToNot(BeNil())
// 			Expect(err.Error()).To(ContainSubstring("no such host"))
// 			Expect(err.Error()).To(ContainSubstring("Get"))

// 			_, err = mover.syncthing.jsonRequest("/rest/config", "GET", nil)
// 			Expect(err).ToNot(BeNil())
// 			Expect(err.Error()).To(ContainSubstring("no such host"))
// 			Expect(err.Error()).To(ContainSubstring("Get"))
// 		})
// 	})

// 	When("Syncthing server exists", func() {
// 		var ts *httptest.Server
// 		var serverSyncthingConfig Config
// 		var sStatus SystemStatus
// 		var sConnections SystemConnections
// 		var apiKeys *corev1.Secret
// 		var myID string = "test"

// 		BeforeEach(func() {
// 			// initialize the config variables here
// 			serverSyncthingConfig = Config{}
// 			sStatus = SystemStatus{}
// 			sConnections = SystemConnections{}
// 		})

// 		JustBeforeEach(func() {
// 			// set status to 10
// 			serverSyncthingConfig.Version = 10

// 			// set our ID
// 			sStatus.MyID = myID

// 			// set information about our connections
// 			sConnections.Total = TotalStats{At: "test"}

// 			ts = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 				switch r.URL.Path {
// 				case "/rest/config":
// 					if r.Method == "GET" {
// 						resBytes, _ := json.Marshal(serverSyncthingConfig)
// 						fmt.Fprintln(w, string(resBytes))
// 					} else if r.Method == "PUT" {
// 						err := json.NewDecoder(r.Body).Decode(&serverSyncthingConfig)
// 						if err != nil {
// 							http.Error(w, "Error decoding request body", http.StatusBadRequest)
// 							return
// 						}
// 					}
// 					return
// 				case "/rest/system/status":
// 					res := sStatus
// 					resBytes, _ := json.Marshal(res)
// 					fmt.Fprintln(w, string(resBytes))
// 					return
// 				case "/rest/system/connections":
// 					res := sConnections
// 					resBytes, _ := json.Marshal(res)
// 					fmt.Fprintln(w, string(resBytes))
// 					return
// 				default:
// 					return
// 				}
// 			}))

// 			mover.syncthing.APIConfig.APIURL = ts.URL
// 			mover.syncthing.APIConfig.Client = ts.Client()
// 			mover.syncthing.APIConfig.APIKey = "test"

// 			// create apikeys secret here so we can use it in the tests
// 			apiKeys = &corev1.Secret{
// 				ObjectMeta: metav1.ObjectMeta{
// 					Name:      "volsync-" + mover.owner.GetName(),
// 					Namespace: ns.Name,
// 				},
// 				Data: map[string][]byte{
// 					"apikey":   []byte("my-secret-apikey-do-not-steal"),
// 					"username": []byte("gcostanza"),
// 					"password": []byte("bosco"),
// 				},
// 			}
// 		})

// 		JustAfterEach(func() {
// 			ts.Close()
// 		})

// 		It("Fetches the Latest Info", func() {
// 			err := mover.syncthing.FetchLatestInfo()
// 			Expect(err).To(BeNil())
// 			Expect(mover.syncthing.Config.Version).To(Equal(10))
// 			Expect(mover.syncthing.SystemStatus.MyID).To(Equal("test"))
// 			Expect(mover.syncthing.SystemConnections.Total.At).To(Equal("test"))
// 		})

// 		It("Updates the Syncthing Config", func() {
// 			mover.syncthing.Config = &Config{
// 				Version: 9,
// 			}
// 			err := mover.syncthing.UpdateSyncthingConfig()
// 			Expect(err).To(BeNil())
// 			Expect(serverSyncthingConfig.Version).To(Equal(9))
// 		})

// 		It("Ensures it's configured", func() {
// 			mover.peerList = []volsyncv1alpha1.SyncthingPeer{
// 				{
// 					Address: "/tcp/127.0.0.1/22000",
// 					ID:      "peer1",
// 				},
// 				{
// 					Address: "/tcp/127.0.0.2/22000",
// 					ID:      "peer2",
// 				},
// 			}

// 			err := mover.ensureIsConfigured(apiKeys)
// 			Expect(err).To(BeNil())

// 			for i, peer := range mover.peerList {
// 				expected := Device{
// 					DeviceID:   peer.ID,
// 					Addresses:  []string{peer.Address},
// 					Introducer: peer.Introducer,
// 				}
// 				Expect(serverSyncthingConfig.Devices[i]).To(Equal(expected))
// 			}
// 		})

// 		It("Ensures the status is updated", func() {
// 			service := &corev1.Service{
// 				ObjectMeta: metav1.ObjectMeta{
// 					Name:      mover.getAPIServiceName(),
// 					Namespace: mover.owner.GetNamespace(),
// 				},
// 				Spec: corev1.ServiceSpec{
// 					ClusterIP: "0.0.0.0",
// 				},
// 			}

// 			mover.peerList = []volsyncv1alpha1.SyncthingPeer{
// 				{
// 					Address: "/tcp/127.0.0.1/22000",
// 					ID:      "peer1",
// 				},
// 				{
// 					Address: "/tcp/127.0.0.2/22000",
// 					ID:      "peer2",
// 				},
// 			}

// 			err := mover.ensureStatusIsUpdated(service)
// 			Expect(err).To(BeNil())

// 			for i, peer := range mover.status.Peers {
// 				p := volsyncv1alpha1.SyncthingPeer{
// 					Address: peer.Address,
// 					ID:      peer.ID,
// 				}
// 				Expect(mover.peerList[i]).To(Equal(p))
// 			}
// 		})

// 		It("jsonRequests without errors", func() {
// 			_, err := mover.syncthing.jsonRequest("/rest/config", "GET", nil)
// 			Expect(err).To(BeNil())

// 			_, err = mover.syncthing.jsonRequest("/rest/system/status", "GET", nil)
// 			Expect(err).To(BeNil())

// 			_, err = mover.syncthing.jsonRequest("/rest/system/connections", "GET", nil)
// 			Expect(err).To(BeNil())
// 		})

// 		When("Syncthing has active connections", func() {

// 			JustBeforeEach(func() {
// 				// add some connections here
// 				sConnections.Connections = map[string]ConnectionStats{
// 					myID: {
// 						Connected: false,
// 						Address:   "",
// 					},
// 					"another-one": {
// 						Connected: true,
// 						Address:   "not-a-real-server",
// 					},
// 				}

// 				// ensure that another-one is in the global config
// 				serverSyncthingConfig.Devices = []Device{
// 					{
// 						DeviceID:     "another-one",
// 						Addresses:    []string{"not-a-real-server"},
// 						Name:         "not-a-real-server",
// 						IntroducedBy: "george-costanza",
// 					},
// 					{
// 						DeviceID:  myID,
// 						Addresses: []string{""},
// 					},
// 				}
// 			})

// 			It("adds them to VolSync status", func() {
// 				// create a data service that Syncthing will use in status updating
// 				fakeDataSVC := corev1.Service{
// 					ObjectMeta: metav1.ObjectMeta{
// 						Name:      "volsync-" + mover.owner.GetName() + "-data",
// 						Namespace: ns.Namespace,
// 					},
// 					Spec: corev1.ServiceSpec{
// 						ClusterIP: "1.2.3.4",
// 						Type:      corev1.ServiceTypeClusterIP,
// 					},
// 				}

// 				// expect status to be updated
// 				err := mover.ensureStatusIsUpdated(&fakeDataSVC)
// 				Expect(err).To(BeNil())

// 				// check that the status contains the new connection
// 				Expect(mover.status.Peers).To(HaveLen(1))
// 				peer := mover.status.Peers[0]

// 				// ensure that volsync properly set these fields
// 				// ID should be the other peer's; not the local one
// 				Expect(peer.ID).To(Equal("another-one"))
// 				Expect(peer.Address).To(Equal("tcp://not-a-real-server"))
// 				Expect(peer.Connected).To(BeTrue())
// 				Expect(peer.IntroducedBy).To(Equal("george-costanza"))
// 				Expect(peer.Name).To(Equal("not-a-real-server"))
// 			})
// 		})

// 		Context("VolSync is improperly configuring Syncthing", func() {
// 			JustBeforeEach(func() {
// 			})

// 			When("VolSync adds its own Syncthing instance to the mover's peerList", func() {
// 				It("errors", func() {
// 					// set the peerlist to itself
// 					mover.peerList = []volsyncv1alpha1.SyncthingPeer{
// 						{
// 							ID:      myID,
// 							Address: "/ip4/127.0.0.1/tcp/22000",
// 						},
// 					}
// 					err := mover.ensureIsConfigured(apiKeys)
// 					Expect(err).To(HaveOccurred())
// 				})
// 			})

// 			When("VolSync tries to add already-introduced peers to Syncthing", func() {
// 				var introducedPeerID string
// 				JustBeforeEach(func() {
// 					// set Syncthing to have an introduced peer
// 					introducedPeerID = "pied-piper"
// 					mover.syncthing.Config.Devices = []Device{
// 						{
// 							DeviceID:     introducedPeerID,
// 							Addresses:    []string{"/ip4/127.0.0.1/tcp/22000"},
// 							Name:         "introduced device",
// 							Introducer:   true,
// 							IntroducedBy: "hooli",
// 						},
// 					}
// 					err := mover.syncthing.UpdateSyncthingConfig()
// 					Expect(err).To(BeNil())

// 				})

// 				It("errors", func() {
// 					// set the peerlist to itself
// 					mover.peerList = []volsyncv1alpha1.SyncthingPeer{
// 						{
// 							ID:      introducedPeerID,
// 							Address: "/ip4/127.0.0.1/tcp/22000",
// 						},
// 					}
// 					err := mover.ensureIsConfigured(apiKeys)
// 					Expect(err).To(HaveOccurred())
// 				})
// 			})
// 		})
// 	})
// })
