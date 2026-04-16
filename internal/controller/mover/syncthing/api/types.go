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
	"crypto/tls"
	"net/http"

	"github.com/backube/volsync/internal/controller/mover/syncthing/lib/config"
)

// SystemStatus contains the fields from /rest/system/status that the mover uses.
// The full response includes many more fields (alloc, cpuPercent, goroutines, etc.)
// which Go's JSON decoder silently ignores.
type SystemStatus struct {
	MyID string `json:"myID"`
}

// ConnectionStats contains the per-device fields from /rest/system/connections
// that the mover uses. The full response includes additional fields
// (paused, clientVersion, type, etc.) which Go's JSON decoder silently ignores.
type ConnectionStats struct {
	Connected bool   `json:"connected"`
	Address   string `json:"address"`
}

// SystemConnections contains the fields from /rest/system/connections that the mover uses.
type SystemConnections struct {
	Connections map[string]ConnectionStats `json:"connections"`
}

// APIConfig Describes the necessary elements needed to configure a client
// with the Syncthing API, included the credentials, URL, TLS Certs.
// This requires nolint:revive because the package it's in is called "api,"
// and it's meant to be used in an interface which already contains `Config`
// meaning a different thing.
// nolint:revive
type APIConfig struct {
	APIURL string `json:"apiURL"`
	APIKey string `json:"apiKey"`
	// don't marshal this field
	TLSConfig *tls.Config
	Client    *http.Client
}

type SyncthingConnection interface {
	// Fetch retrieves the latest configuration, system status, and connections from the Syncthing API.
	Fetch() (*Syncthing, error)
	// AddOrUpdateDevice adds a new device or updates an existing one via POST /rest/config/devices.
	AddOrUpdateDevice(device config.DeviceConfiguration) error
	// RemoveDevice removes a device by ID via DELETE /rest/config/devices/{id}.
	RemoveDevice(deviceID string) error
	// PatchFolderDevices updates only the devices list on a folder via PATCH /rest/config/folders/{id}.
	PatchFolderDevices(folderID string, devices []config.FolderDeviceConfiguration) error
	// PatchGUI updates only the user and password on the GUI config via PATCH /rest/config/gui.
	PatchGUI(user, password string) error
}

// Syncthing Defines a Syncthing API object which contains a subset of the information
// exposed through Syncthing's API. Namely, this struct exposes the configuration,
// system status, and connections contained by the given object.
type Syncthing struct {
	Configuration     config.Configuration
	SystemConnections SystemConnections
	SystemStatus      SystemStatus
}
