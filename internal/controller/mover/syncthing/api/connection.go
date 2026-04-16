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
	"time"

	"github.com/go-logr/logr"

	"github.com/backube/volsync/internal/controller/mover/syncthing/lib/config"
)

// Defines endpoints for the Syncthing API
const (
	SystemStatusEndpoint      = "/rest/system/status"
	SystemConnectionsEndpoint = "/rest/system/connections"
	ConfigEndpoint            = "/rest/config"
	ConfigDevicesEndpoint     = "/rest/config/devices"
	ConfigFoldersEndpoint     = "/rest/config/folders"
	ConfigGUIEndpoint         = "/rest/config/gui"
)

// Fetch Pulls all of Syncthing's latest information from the API and stores it
// in the object's local storage.
func (s *syncthingAPIConnection) Fetch() (*Syncthing, error) {
	// get & store config
	conf, err := s.fetchConfig()
	if err != nil {
		return nil, err
	}

	// get & store connection info
	systemConnections, err := s.fetchSystemConnections()
	if err != nil {
		return nil, err
	}

	// get and store system status
	systemStatus, err := s.fetchSystemStatus()
	if err != nil {
		return nil, err
	}

	return &Syncthing{
		Configuration:     *conf,
		SystemConnections: *systemConnections,
		SystemStatus:      *systemStatus,
	}, nil
}

// AddOrUpdateDevice adds or updates a single device via POST /rest/config/devices.
// Syncthing fills in defaults for any fields not provided.
func (s *syncthingAPIConnection) AddOrUpdateDevice(device config.DeviceConfiguration) error {
	s.logger.Info("Adding/updating Syncthing device", "deviceID", device.DeviceID)
	_, err := s.jsonRequest(ConfigDevicesEndpoint, "POST", device)
	if err != nil {
		s.logger.Error(err, "Failed to add/update device")
	}
	return err
}

// RemoveDevice removes a device by ID via DELETE /rest/config/devices/{id}.
func (s *syncthingAPIConnection) RemoveDevice(deviceID string) error {
	s.logger.Info("Removing Syncthing device", "deviceID", deviceID)
	_, err := s.jsonRequest(ConfigDevicesEndpoint+"/"+deviceID, "DELETE", nil)
	if err != nil {
		s.logger.Error(err, "Failed to remove device")
	}
	return err
}

// PatchFolderDevices updates only the devices list on a folder via PATCH /rest/config/folders/{id}.
// All other folder settings are preserved server-side.
func (s *syncthingAPIConnection) PatchFolderDevices(
	folderID string, devices []config.FolderDeviceConfiguration,
) error {
	s.logger.Info("Updating folder device sharing", "folderID", folderID)
	body := struct {
		Devices []config.FolderDeviceConfiguration `json:"devices"`
	}{Devices: devices}
	_, err := s.jsonRequest(ConfigFoldersEndpoint+"/"+folderID, "PATCH", body)
	if err != nil {
		s.logger.Error(err, "Failed to update folder devices")
	}
	return err
}

// PatchGUI updates only the user and password on the GUI config via PATCH /rest/config/gui.
// All other GUI settings are preserved server-side.
func (s *syncthingAPIConnection) PatchGUI(user, password string) error {
	s.logger.Info("Updating Syncthing GUI credentials")
	body := struct {
		User     string `json:"user"`
		Password string `json:"password"`
	}{User: user, Password: password}
	_, err := s.jsonRequest(ConfigGUIEndpoint, "PATCH", body)
	if err != nil {
		s.logger.Error(err, "Failed to update GUI credentials")
	}
	return err
}

// NewConnection accepts an APIConfig object and a logger and creates a SyncthingConnection
// object in return.
func NewConnection(cfg APIConfig, logger logr.Logger) SyncthingConnection {
	return &syncthingAPIConnection{
		apiConfig: cfg,
		logger:    logger,
	}
}

// TLSClient Returns a TLS Client used by the API Config.
// If the client field is nil, then a new TLS Client is built using
// either the custom TLS Config set or a default tlsConfig with version 1.2
func (api APIConfig) TLSClient() *http.Client {
	if api.Client != nil {
		return api.Client
	}

	tlsConfig := api.TLSConfig
	if tlsConfig == nil {
		tlsConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}

	// load the TLS config with certificates
	tr := &http.Transport{
		TLSClientConfig: tlsConfig,
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   time.Second * 5,
	}
	return client
}
