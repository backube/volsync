//go:build !disable_syncthing

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

//nolint:revive
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/backube/volsync/internal/controller/mover/syncthing/lib/config"
)

// GetDeviceFromID Returns a pointer to the device with the given ID,
// along with a boolean indicating whether the device was found.
func (s *Syncthing) GetDeviceFromID(id string) (*config.DeviceConfiguration, bool) {
	for _, device := range s.Configuration.Devices {
		if device.DeviceID == id {
			return &device, true
		}
	}
	return nil, false
}

// MyID Is a convenience method which returns the current Syncthing device's ID.
func (s *Syncthing) MyID() string { return s.SystemStatus.MyID }

// CreateSyncthingTestServer Returns a test server that mimics the Syncthing API by exposing
// the endpoints for config, system status, system connections, and granular config updates.
// The server also accepts an API Key, which is used for authenticating between the client and server.
//
// The accepted arguments are pointers so that the state can be changed externally and the server
// will be updated accordingly.
//
//nolint:funlen,cyclop
func CreateSyncthingTestServer(state *Syncthing, serverAPIKey string) *httptest.Server {
	setConnections := func(s *Syncthing) {
		connections := make(map[string]ConnectionStats, 0)
		for _, device := range s.Configuration.Devices {
			if len(device.Addresses) > 0 {
				connections[device.DeviceID] = ConnectionStats{
					Connected: true,
					Address:   device.Addresses[0],
				}
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

		path := r.URL.Path

		// POST /rest/config/devices — add/update a single device
		if path == ConfigDevicesEndpoint && r.Method == "POST" {
			var device config.DeviceConfiguration
			if err := json.NewDecoder(r.Body).Decode(&device); err != nil {
				http.Error(w, "Error decoding request body", http.StatusBadRequest)
				return
			}
			// Update existing or append new
			found := false
			for i, d := range state.Configuration.Devices {
				if d.DeviceID == device.DeviceID {
					state.Configuration.Devices[i] = device
					found = true
					break
				}
			}
			if !found {
				state.Configuration.Devices = append(state.Configuration.Devices, device)
			}
			setConnections(state)
			return
		}

		// DELETE /rest/config/devices/{id} — remove a device
		if strings.HasPrefix(path, ConfigDevicesEndpoint+"/") && r.Method == "DELETE" {
			deviceID := strings.TrimPrefix(path, ConfigDevicesEndpoint+"/")
			newDevices := make([]config.DeviceConfiguration, 0, len(state.Configuration.Devices))
			for _, d := range state.Configuration.Devices {
				if d.DeviceID != deviceID {
					newDevices = append(newDevices, d)
				}
			}
			state.Configuration.Devices = newDevices
			setConnections(state)
			return
		}

		// PATCH /rest/config/folders/{id} — update folder fields (devices list)
		if strings.HasPrefix(path, ConfigFoldersEndpoint+"/") && r.Method == "PATCH" {
			folderID := strings.TrimPrefix(path, ConfigFoldersEndpoint+"/")
			var patch struct {
				Devices []config.FolderDeviceConfiguration `json:"devices"`
			}
			if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
				http.Error(w, "Error decoding request body", http.StatusBadRequest)
				return
			}
			for i, f := range state.Configuration.Folders {
				if f.ID == folderID {
					state.Configuration.Folders[i].Devices = patch.Devices
					break
				}
			}
			return
		}

		// PATCH /rest/config/gui — update GUI fields
		if path == ConfigGUIEndpoint && r.Method == "PATCH" {
			var patch struct {
				User     string `json:"user"`
				Password string `json:"password"`
			}
			if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
				http.Error(w, "Error decoding request body", http.StatusBadRequest)
				return
			}
			if patch.User != "" {
				state.Configuration.GUI.User = patch.User
			}
			if patch.Password != "" {
				state.Configuration.GUI.Password = patch.Password
			}
			return
		}

		// GET /rest/config — read full config
		if path == ConfigEndpoint && r.Method == "GET" {
			resBytes, _ := json.Marshal(state.Configuration)
			fmt.Fprintln(w, string(resBytes))
			return
		}

		// GET /rest/system/status
		if path == SystemStatusEndpoint {
			resBytes, _ := json.Marshal(state.SystemStatus)
			fmt.Fprintln(w, string(resBytes))
			return
		}

		// GET /rest/system/connections
		if path == SystemConnectionsEndpoint {
			resBytes, _ := json.Marshal(state.SystemConnections)
			fmt.Fprintln(w, string(resBytes))
			return
		}

		// the endpoint doesn't exist
		http.Error(w, "the resource path doesn't exist", http.StatusNotFound)
	}))
}
