//go:build !disable_syncthing

//nolint:revive
package api

import (
	"net/http"
	"net/http/httptest"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/backube/volsync/internal/controller/mover/syncthing/lib/config"
)

var _ = Describe("Syncthing connection", func() {

	Context("Syncthing API is being used properly", func() {

		When("Syncthing server exists", func() {
			var (
				ts           *httptest.Server
				serverState  *Syncthing
				myID         = "ZNWFSWE-RWRV2BD-45BLMCV-LTDE2UR-4LJDW6J-R5BPWEB-TXD27XJ-IZF5RA4"
				device1      = "AIR6LPZ-7K4PTTV-UXQSMUU-CPQ5YWH-OEDFIIQ-JUG777G-2YQXXR5-YD6AWQR"
				device2      = "GYRZZQB-IRNPV4Z-T7TC52W-EQYJ3TT-FDQW6MW-DFLMU42-SSSU6EM-FBK2VAY"
				serverAPIKey = "0xDEADBEEF"
			)

			BeforeEach(func() {
				serverState = &Syncthing{}
			})

			JustBeforeEach(func() {
				serverState.SystemStatus.MyID = myID

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
					Expect(syncthing.SystemStatus.MyID).To(Equal(myID))
				})

				It("adds a device via AddOrUpdateDevice", func() {
					err := syncthingConnection.AddOrUpdateDevice(config.DeviceConfiguration{
						DeviceID:  device1,
						Addresses: []string{"tcp://1.2.3.4:22000"}, //nolint:goconst
					})
					Expect(err).ToNot(HaveOccurred())
					Expect(serverState.Configuration.Devices).To(HaveLen(1))
					Expect(serverState.Configuration.Devices[0].DeviceID).To(Equal(device1))
				})

				It("updates an existing device via AddOrUpdateDevice", func() {
					// Add device first
					err := syncthingConnection.AddOrUpdateDevice(config.DeviceConfiguration{
						DeviceID:  device1,
						Addresses: []string{"tcp://1.2.3.4:22000"}, //nolint:goconst
					})
					Expect(err).ToNot(HaveOccurred())

					// Update with new address
					err = syncthingConnection.AddOrUpdateDevice(config.DeviceConfiguration{
						DeviceID:  device1,
						Addresses: []string{"tcp://5.6.7.8:22000"},
					})
					Expect(err).ToNot(HaveOccurred())
					Expect(serverState.Configuration.Devices).To(HaveLen(1))
					Expect(serverState.Configuration.Devices[0].Addresses[0]).To(Equal("tcp://5.6.7.8:22000"))
				})

				It("removes a device via RemoveDevice", func() {
					// Add device first
					err := syncthingConnection.AddOrUpdateDevice(config.DeviceConfiguration{
						DeviceID:  device1,
						Addresses: []string{"tcp://1.2.3.4:22000"}, //nolint:goconst
					})
					Expect(err).ToNot(HaveOccurred())
					Expect(serverState.Configuration.Devices).To(HaveLen(1))

					// Remove it
					err = syncthingConnection.RemoveDevice(device1)
					Expect(err).ToNot(HaveOccurred())
					Expect(serverState.Configuration.Devices).To(BeEmpty())
				})

				It("patches folder devices via PatchFolderDevices", func() {
					// Set up a folder on the server
					serverState.Configuration.Folders = []config.FolderConfiguration{
						{ID: "default", Label: "Default", Devices: []config.FolderDeviceConfiguration{}},
					}

					// Patch the folder's devices
					err := syncthingConnection.PatchFolderDevices("default", []config.FolderDeviceConfiguration{
						{DeviceID: device1},
						{DeviceID: device2},
					})
					Expect(err).ToNot(HaveOccurred())
					Expect(serverState.Configuration.Folders[0].Devices).To(HaveLen(2))
				})

				It("patches GUI credentials via PatchGUI", func() {
					err := syncthingConnection.PatchGUI("admin", "secret123")
					Expect(err).ToNot(HaveOccurred())
					Expect(serverState.Configuration.GUI.User).To(Equal("admin"))
					Expect(serverState.Configuration.GUI.Password).To(Equal("secret123"))
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

				It("jsonRequests without errors", func() {
					// all of these request methods should succeed
					_, err := apiConnection.jsonRequest(ConfigEndpoint, "GET", nil)
					Expect(err).ToNot(HaveOccurred())

					_, err = apiConnection.jsonRequest(SystemStatusEndpoint, "GET", nil)
					Expect(err).ToNot(HaveOccurred())

					_, err = apiConnection.jsonRequest(SystemConnectionsEndpoint, "GET", nil)
					Expect(err).ToNot(HaveOccurred())

					stConfig, err := apiConnection.fetchConfig()
					Expect(err).ToNot(HaveOccurred())
					Expect(stConfig).NotTo(BeNil())

					connections, err := apiConnection.fetchSystemConnections()
					Expect(err).ToNot(HaveOccurred())
					Expect(connections).NotTo(BeNil())

					status, err := apiConnection.fetchSystemStatus()
					Expect(err).ToNot(HaveOccurred())
					Expect(status).NotTo(BeNil())

					syncthingResponse, err := apiConnection.Fetch()
					Expect(err).ToNot(HaveOccurred())
					Expect(syncthingResponse).NotTo(BeNil())
				})

				When("the wrong api key is used", func() {
					JustBeforeEach(func() {
						apiConnection.apiConfig.APIKey = "my-super-secret-key-DO-NOT-STEAL!!!"
					})

					It("errors on all operations", func() {
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

						err = apiConnection.AddOrUpdateDevice(config.DeviceConfiguration{
							DeviceID: device1,
						})
						Expect(err).To(HaveOccurred())

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
		syncthing *Syncthing
		myID      = "ZNWFSWE-RWRV2BD-45BLMCV-LTDE2UR-4LJDW6J-R5BPWEB-TXD27XJ-IZF5RA4"
		device1   = "AIR6LPZ-7K4PTTV-UXQSMUU-CPQ5YWH-OEDFIIQ-JUG777G-2YQXXR5-YD6AWQR"
		device2   = "GYRZZQB-IRNPV4Z-T7TC52W-EQYJ3TT-FDQW6MW-DFLMU42-SSSU6EM-FBK2VAY"
		device3   = "VNPQDOJ-3V7DEWN-QBCTXF2-LSVNMHL-XTGL4GX-NCGQEXQ-THHBVWR-HVVMEQR"
		device4   = "E3TWU3G-UGFHTJE-SJLCDYH-KGQR3R6-7QMOM43-FOC3UFT-H4H54DC-GMK5RAO"
	)

	BeforeEach(func() {
		syncthing = &Syncthing{}
	})

	When("devices are present in Syncthing struct", func() {
		BeforeEach(func() {
			syncthing.Configuration.Devices = []config.DeviceConfiguration{
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
		})

		It("finds the ones that are stored", func() {
			devices := []struct {
				deviceID   string
				shouldFind bool
			}{
				{deviceID: device1, shouldFind: true},
				{deviceID: device2, shouldFind: true},
				{deviceID: device3, shouldFind: true},
				{deviceID: device4, shouldFind: false},
			}
			for _, device := range devices {
				config, ok := syncthing.GetDeviceFromID(device.deviceID)
				if device.shouldFind {
					Expect(ok).To(BeTrue())
					Expect(config).NotTo(BeNil())
				} else {
					Expect(ok).NotTo(BeTrue())
					Expect(config).To(BeNil())
				}
			}
		})
	})

	It("returns the ID when set", func() {
		// ID present should return a string
		syncthing.SystemStatus.MyID = myID
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
