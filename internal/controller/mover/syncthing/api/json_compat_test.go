//go:build !disable_syncthing

//nolint:revive
package api

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/backube/volsync/internal/controller/mover/syncthing/lib/config"
)

// These tests validate that our struct JSON tags match the actual Syncthing REST API
// field names. The fixtures below are based on Syncthing v1.x REST API documentation
// (https://docs.syncthing.net/rest/).
//
// If Syncthing changes its API field names (e.g., in v2), these tests will catch it
// when the fixtures are updated to match the new API.
var _ = Describe("JSON compatibility with Syncthing REST API", func() {

	Describe("GET /rest/config", func() {
		// Fixture based on Syncthing REST API docs.
		// Extra fields from the real API (ldap, options, defaults, etc.) are included
		// to verify our structs tolerate unknown fields.
		const configJSON = `{
			"version": 37,
			"folders": [{
				"id": "default",
				"label": "Default Folder",
				"filesystemType": "basic",
				"path": "/data",
				"type": "sendreceive",
				"devices": [
					{
						"deviceID": "P56IOI7-MZJNU2Y-IQGDREY-DM2MGTI-MGL3BXN-PQ6W5BM-TBBZ4TJ-XZWICQ2",
						"introducedBy": "",
						"encryptionPassword": ""
					}
				],
				"rescanIntervalS": 3600,
				"fsWatcherEnabled": true,
				"ignorePerms": false,
				"autoNormalize": true
			}],
			"devices": [{
				"deviceID": "P56IOI7-MZJNU2Y-IQGDREY-DM2MGTI-MGL3BXN-PQ6W5BM-TBBZ4TJ-XZWICQ2",
				"name": "node1",
				"addresses": ["dynamic"],
				"compression": "metadata",
				"introducer": false,
				"skipIntroductionRemovals": false,
				"introducedBy": "",
				"paused": false,
				"autoAcceptFolders": false,
				"maxSendKbps": 0,
				"maxRecvKbps": 0,
				"numConnections": 0
			}],
			"gui": {
				"enabled": true,
				"address": "127.0.0.1:8384",
				"user": "admin",
				"password": "$2a$10$abcdefghijklmnopqrstuv",
				"authMode": "static",
				"useTLS": false,
				"apiKey": "abc123",
				"theme": "default"
			},
			"ldap": {},
			"options": {},
			"remoteIgnoredDevices": [],
			"defaults": {}
		}`

		It("unmarshals into our Configuration struct preserving fields we use", func() {
			var cfg config.Configuration
			err := json.Unmarshal([]byte(configJSON), &cfg)
			Expect(err).NotTo(HaveOccurred())

			// Devices
			Expect(cfg.Devices).To(HaveLen(1))
			Expect(cfg.Devices[0].Name).To(Equal("node1"))
			Expect(cfg.Devices[0].Addresses).To(ConsistOf("dynamic"))
			Expect(cfg.Devices[0].DeviceID).To(Equal(
				"P56IOI7-MZJNU2Y-IQGDREY-DM2MGTI-MGL3BXN-PQ6W5BM-TBBZ4TJ-XZWICQ2"))
			Expect(cfg.Devices[0].Introducer).To(BeFalse())
			Expect(cfg.Devices[0].IntroducedBy).To(Equal(""))

			// Folders
			Expect(cfg.Folders).To(HaveLen(1))
			Expect(cfg.Folders[0].ID).To(Equal("default"))
			Expect(cfg.Folders[0].Label).To(Equal("Default Folder"))
			Expect(cfg.Folders[0].Path).To(Equal("/data"))
			Expect(cfg.Folders[0].Devices).To(HaveLen(1))
			Expect(cfg.Folders[0].Devices[0].DeviceID).To(Equal(
				"P56IOI7-MZJNU2Y-IQGDREY-DM2MGTI-MGL3BXN-PQ6W5BM-TBBZ4TJ-XZWICQ2"))

			// GUI
			Expect(cfg.GUI.User).To(Equal("admin"))
			Expect(cfg.GUI.Password).To(HavePrefix("$2a$10$"))
		})

		It("roundtrips our Configuration through marshal/unmarshal", func() {
			var cfg config.Configuration
			err := json.Unmarshal([]byte(configJSON), &cfg)
			Expect(err).NotTo(HaveOccurred())

			data, err := json.Marshal(cfg)
			Expect(err).NotTo(HaveOccurred())

			var cfg2 config.Configuration
			err = json.Unmarshal(data, &cfg2)
			Expect(err).NotTo(HaveOccurred())

			Expect(cfg2.Devices).To(HaveLen(len(cfg.Devices)))
			Expect(cfg2.Folders).To(HaveLen(len(cfg.Folders)))
			Expect(cfg2.Devices[0].DeviceID).To(Equal(cfg.Devices[0].DeviceID))
		})

		It("produces JSON with correct field names", func() {
			var cfg config.Configuration
			err := json.Unmarshal([]byte(configJSON), &cfg)
			Expect(err).NotTo(HaveOccurred())

			data, err := json.Marshal(cfg)
			Expect(err).NotTo(HaveOccurred())

			var raw map[string]json.RawMessage
			err = json.Unmarshal(data, &raw)
			Expect(err).NotTo(HaveOccurred())

			// Verify top-level field names match Syncthing API
			Expect(raw).To(HaveKey("folders"))
			Expect(raw).To(HaveKey("devices"))
			Expect(raw).To(HaveKey("gui"))
		})
	})

	Describe("GET /rest/system/status", func() {
		const systemStatusJSON = `{
			"alloc": 30618136,
			"connectionServiceStatus": {
				"tcp://0.0.0.0:22000": {
					"error": null,
					"lanAddresses": ["tcp://192.168.1.2:22000"],
					"wanAddresses": ["tcp://1.2.3.4:22000"]
				}
			},
			"cpuPercent": 2,
			"discoveryEnabled": true,
			"discoveryMethods": 4,
			"goroutines": 49,
			"guiAddressOverridden": false,
			"guiAddressUsed": "127.0.0.1:8384",
			"lastDialStatus": {
				"tcp://192.168.1.3:22000": {
					"when": "2023-01-01T00:00:00Z",
					"error": null,
					"ok": true
				}
			},
			"myID": "P56IOI7-MZJNU2Y-IQGDREY-DM2MGTI-MGL3BXN-PQ6W5BM-TBBZ4TJ-XZWICQ2",
			"pathSeparator": "/",
			"startTime": "2023-01-01T00:00:00Z",
			"uptime": 3600
		}`

		It("unmarshals into our SystemStatus struct extracting myID", func() {
			var status SystemStatus
			err := json.Unmarshal([]byte(systemStatusJSON), &status)
			Expect(err).NotTo(HaveOccurred())

			Expect(status.MyID).To(Equal(
				"P56IOI7-MZJNU2Y-IQGDREY-DM2MGTI-MGL3BXN-PQ6W5BM-TBBZ4TJ-XZWICQ2"))
		})
	})

	Describe("GET /rest/system/connections", func() {
		const connectionsJSON = `{
			"connections": {
				"YZJBJFX-RDBL7WY-6ZGKJ2D-4MJB4E7-ZATSDUY-LD6Y3L3-MLFUYWE-AEMXJAC": {
					"at": "2015-11-07T17:29:47.691548971+01:00",
					"inBytesTotal": 556,
					"outBytesTotal": 550,
					"connected": true,
					"paused": false,
					"startedAt": "2015-11-07T00:09:47Z",
					"clientVersion": "v1.30.0",
					"address": "127.0.0.1:22002",
					"type": "tcp-client",
					"isLocal": true
				}
			},
			"total": {
				"at": "2015-11-07T17:29:47.691637262+01:00",
				"inBytesTotal": 1479,
				"outBytesTotal": 1318
			}
		}`

		It("unmarshals into our SystemConnections struct", func() {
			var conn SystemConnections
			err := json.Unmarshal([]byte(connectionsJSON), &conn)
			Expect(err).NotTo(HaveOccurred())

			// Per-device connection
			deviceKey := "YZJBJFX-RDBL7WY-6ZGKJ2D-4MJB4E7-ZATSDUY-LD6Y3L3-MLFUYWE-AEMXJAC"
			Expect(conn.Connections).To(HaveKey(deviceKey))
			c := conn.Connections[deviceKey]
			Expect(c.Connected).To(BeTrue())
			Expect(c.Address).To(Equal("127.0.0.1:22002"))
		})
	})
})
