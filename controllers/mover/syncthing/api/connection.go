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
	"crypto/tls"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"github.com/syncthing/syncthing/lib/config"
)

// Defines endpoints for the Syncthing API
const (
	SystemStatusEndpoint      = "/rest/system/status"
	SystemConnectionsEndpoint = "/rest/system/connections"
	ConfigEndpoint            = "/rest/config"
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

// PublishConfig Updates the Syncthing API with the stored configuration data.
// An error is returned in the case of a failure.
func (s *syncthingAPIConnection) PublishConfig(conf config.Configuration) error {
	// update the config
	s.logger.Info("Updating Syncthing config")
	_, err := s.jsonRequest(ConfigEndpoint, "PUT", conf)
	if err != nil {
		s.logger.Error(err, "Failed to update Syncthing config")
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
