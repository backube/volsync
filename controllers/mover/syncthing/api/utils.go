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
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/syncthing/syncthing/lib/config"
)

// GetDeviceFromID Returns a pointer to the device with the given ID,
// along with a boolean indicating whether the device was found.
func (s *Syncthing) GetDeviceFromID(id string) (*config.DeviceConfiguration, bool) {
	for _, device := range s.Configuration.Devices {
		if device.DeviceID.GoString() == id {
			return &device, true
		}
	}
	return nil, false
}

// MyID Is a convenience method which returns the current Syncthing device's ID.
func (s *Syncthing) MyID() string { return s.SystemStatus.MyID }

// ShareFoldersWithDevices Will set all of the devices in s.Configuration.Devices to be shared with the
// currently tracked folders.
//
// This method does not currently take into account any encryption password set
// on the folder by the device.
func (s *Syncthing) ShareFoldersWithDevices() {
	// share the current folder(s) with the new devices
	var newFolders = []config.FolderConfiguration{}
	for _, folder := range s.Configuration.Folders {
		// copy folder & reset
		newFolder := folder
		newFolder.Devices = []config.FolderDeviceConfiguration{}

		for _, device := range s.Configuration.Devices {
			newFolder.Devices = append(newFolder.Devices, config.FolderDeviceConfiguration{
				DeviceID:     device.DeviceID,
				IntroducedBy: device.IntroducedBy,
			})
		}
		newFolders = append(newFolders, newFolder)
	}
	s.Configuration.Folders = newFolders
}

// CreateSyncthingTestServer Returns a test server that mimics the Syncthing API by exposing
// the endpoints for config, system status, and system connections.
// The server also accepts an API Key, which is used for authenticating between the client and server.
//
// The accepted arguments are pointers so that the state can be changed externally and the server
// will be updated accordingly.
// nolint:funlen
func CreateSyncthingTestServer(state *Syncthing, serverAPIKey string) *httptest.Server {
	setConnections := func(s *Syncthing) {
		connections := make(map[string]ConnectionStats, 0)
		for _, device := range s.Configuration.Devices {
			connections[device.DeviceID.GoString()] = ConnectionStats{
				Connected:     true,
				Paused:        false,
				Address:       device.Addresses[0],
				Type:          "TCP",
				ClientVersion: "v1.0.0",
			}
		}
		s.SystemConnections.Connections = connections
	}

	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ensure that the client is authorized
		apiKey := r.Header.Get("X-API-Key")
		if apiKey != serverAPIKey {
			http.Error(w, "Unauthorized client", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case ConfigEndpoint:
			if r.Method == "GET" {
				resBytes, _ := json.Marshal(state.Configuration)
				fmt.Fprintln(w, string(resBytes))
			} else if r.Method == "PUT" {
				err := json.NewDecoder(r.Body).Decode(&state.Configuration)
				if err != nil {
					http.Error(w, "Error decoding request body", http.StatusBadRequest)
					return
				}
				// update the connections
				setConnections(state)
			}
			return
		case SystemStatusEndpoint:
			res := state.SystemStatus
			resBytes, _ := json.Marshal(res)
			fmt.Fprintln(w, string(resBytes))
			return
		case SystemConnectionsEndpoint:
			res := state.SystemConnections
			resBytes, _ := json.Marshal(res)
			fmt.Fprintln(w, string(resBytes))
			return
		default:
			// the endpoint doesn't exist
			http.Error(w, "the resource path doesn't exist", http.StatusNotFound)
			return
		}
	}))
}
