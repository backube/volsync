package api

import (
	"net/http"
	"net/http/httptest"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/protocol"
)

var _ = Describe("Syncthing connection", func() {

	Context("Syncthing API is being used properly", func() {

		When("Syncthing server exists", func() {
			var (
				ts          *httptest.Server
				serverState *Syncthing
				myID, _     = protocol.DeviceIDFromString(
					"ZNWFSWE-RWRV2BD-45BLMCV-LTDE2UR-4LJDW6J-R5BPWEB-TXD27XJ-IZF5RA4",
				)
				serverAPIKey = "0xDEADBEEF"
			)

			BeforeEach(func() {
				serverState = &Syncthing{}
			})

			JustBeforeEach(func() {
				// set a value in each field
				serverState.Configuration.Version = 10
				serverState.SystemStatus.MyID = myID.GoString()
				serverState.SystemConnections.Total = TotalStats{At: "test"}

				ts = CreateSyncthingTestServer(serverState, serverAPIKey)
			})

			JustAfterEach(func() {
				ts.Close()
			})

			When("syncthingConnection interface is used", func() {
				var syncthingConnection SyncthingConnection

				JustBeforeEach(func() {
					// create a syncthing connection
					apiConfig := APIConfig{
						APIURL: ts.URL,
						APIKey: serverAPIKey,
						Client: ts.Client(),
					}
					syncthingConnection = NewConnection(apiConfig, logr.Discard().WithName("syncthing-api"))

				})

				It("fetches the Latest Info", func() {
					syncthing, err := syncthingConnection.Fetch()
					Expect(err).NotTo(HaveOccurred())
					Expect(syncthing).NotTo(BeNil())

					// ensure that we fetched the server's values
					Expect(syncthing.Configuration.Version).To(Equal(10))
					Expect(syncthing.SystemStatus.MyID).To(Equal(myID.GoString()))
					Expect(syncthing.SystemConnections.Total.At).To(Equal("test"))
				})

				It("updates the Syncthing Config", func() {
					syncthing := &Syncthing{
						Configuration: config.Configuration{
							Version: 9,
						},
					}

					// write to the server
					err := syncthingConnection.PublishConfig(syncthing.Configuration)
					Expect(err).To(BeNil())
					Expect(serverState.Configuration.Version).To(Equal(9))
				})

			})

			When("syncthingAPIConnection is making requests to the server", func() {
				var apiConnection *syncthingAPIConnection

				JustBeforeEach(func() {
					apiConnection = &syncthingAPIConnection{
						apiConfig: APIConfig{
							APIURL: ts.URL,
							APIKey: serverAPIKey,
							Client: ts.Client(),
						},
						logger: logr.Discard().WithName("api"),
					}
				})

				// nolint:dupl
				It("jsonRequests without errors", func() {
					// all of these request methods should succeed
					_, err := apiConnection.jsonRequest(ConfigEndpoint, "GET", nil)
					Expect(err).To(BeNil())

					_, err = apiConnection.jsonRequest(SystemStatusEndpoint, "GET", nil)
					Expect(err).To(BeNil())

					_, err = apiConnection.jsonRequest(SystemConnectionsEndpoint, "GET", nil)
					Expect(err).To(BeNil())

					stConfig, err := apiConnection.fetchConfig()
					Expect(err).NotTo(HaveOccurred())
					Expect(stConfig).NotTo(BeNil())

					connections, err := apiConnection.fetchSystemConnections()
					Expect(err).NotTo(HaveOccurred())
					Expect(connections).NotTo(BeNil())

					status, err := apiConnection.fetchSystemStatus()
					Expect(err).NotTo(HaveOccurred())
					Expect(status).NotTo(BeNil())

					mockConfig := config.Configuration{Version: 74}
					err = apiConnection.PublishConfig(mockConfig)
					Expect(err).NotTo(HaveOccurred())
					Expect(serverState.Configuration.Version).To(Equal(mockConfig.Version))

					syncthingResponse, err := apiConnection.Fetch()
					Expect(err).NotTo(HaveOccurred())
					Expect(syncthingResponse).NotTo(BeNil())

				})

				When("the wrong api key is used", func() {
					JustBeforeEach(func() {
						apiConnection.apiConfig.APIKey = "my-super-secret-key-DO-NOT-STEAL!!!"
					})

					// nolint:dupl
					It("errors", func() {
						// ensure all of the api methods & helpers error here
						_, err := apiConnection.jsonRequest(ConfigEndpoint, "GET", nil)
						Expect(err).To(HaveOccurred())

						_, err = apiConnection.jsonRequest(SystemStatusEndpoint, "GET", nil)
						Expect(err).To(HaveOccurred())

						_, err = apiConnection.jsonRequest(SystemConnectionsEndpoint, "GET", nil)
						Expect(err).To(HaveOccurred())

						stConfig, err := apiConnection.fetchConfig()
						Expect(err).To(HaveOccurred())
						Expect(stConfig).To(BeNil())

						connections, err := apiConnection.fetchSystemConnections()
						Expect(err).To(HaveOccurred())
						Expect(connections).To(BeNil())

						status, err := apiConnection.fetchSystemStatus()
						Expect(err).To(HaveOccurred())
						Expect(status).To(BeNil())

						mockConfig := config.Configuration{Version: 74}
						err = apiConnection.PublishConfig(mockConfig)
						Expect(err).To(HaveOccurred())
						Expect(serverState.Configuration.Version).NotTo(Equal(mockConfig.Version))

						syncthingResponse, err := apiConnection.Fetch()
						Expect(err).To(HaveOccurred())
						Expect(syncthingResponse).To(BeNil())
					})
				})

				When("the server endpoint doesn't exist", func() {
					It("returns an error", func() {
						_, err := apiConnection.jsonRequest("/this/is/not/a/real/endpoint", "GET", nil)
						Expect(err).To(HaveOccurred())
					})
				})
			})
		})
	})
})

var _ = Describe("Syncthing struct methods", func() {
	var (
		syncthing  *Syncthing
		myID, _    = protocol.DeviceIDFromString("ZNWFSWE-RWRV2BD-45BLMCV-LTDE2UR-4LJDW6J-R5BPWEB-TXD27XJ-IZF5RA4")
		device1, _ = protocol.DeviceIDFromString("AIR6LPZ-7K4PTTV-UXQSMUU-CPQ5YWH-OEDFIIQ-JUG777G-2YQXXR5-YD6AWQR")
		device2, _ = protocol.DeviceIDFromString("GYRZZQB-IRNPV4Z-T7TC52W-EQYJ3TT-FDQW6MW-DFLMU42-SSSU6EM-FBK2VAY")
		device3, _ = protocol.DeviceIDFromString("VNPQDOJ-3V7DEWN-QBCTXF2-LSVNMHL-XTGL4GX-NCGQEXQ-THHBVWR-HVVMEQR")
		device4, _ = protocol.DeviceIDFromString("E3TWU3G-UGFHTJE-SJLCDYH-KGQR3R6-7QMOM43-FOC3UFT-H4H54DC-GMK5RAO")
	)

	BeforeEach(func() {
		syncthing = &Syncthing{}
	})

	When("devices are present in Syncthing struct", func() {
		BeforeEach(func() {
			devices := []config.DeviceConfiguration{
				{
					DeviceID:  device1,
					Name:      "IoT-furnace",
					Addresses: []string{"tcp4://1.2.3.4:22000"},
				},
				{
					DeviceID:  device2,
					Name:      "IoT-fire-alarm",
					Addresses: []string{"tcp6://[:5:ab:1006]:22000"},
				},
				{
					DeviceID:  device3,
					Name:      "IoT-fern",
					Addresses: []string{"udp4://196.168.1.203:23000"},
				},
			}
			syncthing.Configuration.SetDevices(devices)
		})

		It("finds the ones that are stored", func() {
			devices := []struct {
				deviceID   protocol.DeviceID
				shouldFind bool
			}{
				{
					deviceID:   device1,
					shouldFind: true,
				},
				{
					deviceID:   device2,
					shouldFind: true,
				},
				{
					deviceID:   device3,
					shouldFind: true,
				},
				{
					deviceID:   device4,
					shouldFind: false,
				},
			}
			for _, device := range devices {
				config, ok := syncthing.GetDeviceFromID(device.deviceID.GoString())
				if device.shouldFind {
					Expect(ok).To(BeTrue())
					Expect(config).NotTo(BeNil())
				} else {
					Expect(ok).NotTo(BeTrue())
					Expect(config).To(BeNil())
				}
			}
		})

		When("folders are present", func() {
			BeforeEach(func() {
				syncthing.Configuration.SetFolder(config.FolderConfiguration{
					ID:      "b-mitzvah-recordings",
					Label:   "B.Mitzvah Recordings",
					Devices: []config.FolderDeviceConfiguration{},
				})
			})

			It("shares them with the given devices", func() {
				Expect(len(syncthing.Configuration.Folders)).NotTo(BeZero())
				Expect(len(syncthing.Configuration.Folders[0].Devices)).To(BeZero())
				// devices have not been shared yet so expect to find nothing
				syncthing.ShareFoldersWithDevices()
				Expect(len(syncthing.Configuration.Folders[0].Devices)).
					To(Equal(len(syncthing.Configuration.Devices)))

				deviceMap := syncthing.Configuration.DeviceMap()
				sharedWith := syncthing.Configuration.Folders[0].Devices
				for deviceID := range deviceMap {
					found := false
					for _, sharedDevice := range sharedWith {
						if sharedDevice.DeviceID == deviceID {
							found = true
							break
						}
					}
					Expect(found).To(BeTrue())
				}
			})
		})
	})

	It("returns the ID when set", func() {
		// ID present should return a string
		syncthing.SystemStatus.MyID = myID.GoString()
		Expect(syncthing.MyID()).NotTo(Equal(""))

		syncthing.SystemStatus.MyID = ""
		Expect(syncthing.MyID()).To(Equal(""))
	})

})

var _ = Describe("APIConfig", func() {
	var apiConfig *APIConfig
	BeforeEach(func() {
		apiConfig = &APIConfig{}
	})
	When("an HTTP Client is set", func() {
		var httpClient *http.Client
		BeforeEach(func() {
			httpClient = &http.Client{}
			apiConfig.Client = httpClient
		})
		It("uses the existing client", func() {
			// client should be the same as the one created earlier
			client := apiConfig.TLSClient()
			Expect(client).To(Equal(httpClient))

			// clear the current client to show that it makes a new one
			apiConfig.Client = nil
			newClient := apiConfig.TLSClient()
			Expect(newClient).NotTo(Equal(client))
		})
	})
})
