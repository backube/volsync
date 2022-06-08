// syncthing.go Defines the API interface for the Syncthing object
package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/go-logr/logr"
	"github.com/syncthing/syncthing/lib/config"
)

// MyID Returns a string representation of the configured Syncthing device's ID,
// or an Error in the case that system status is not present.
func (s *Syncthing) MyID() string {
	return s.systemStatus.MyID
}

// Config Returns the configuration, or an error if the config is not present.
func (s *Syncthing) Config() config.Configuration {
	return s.configuration
}

// SystemStatus Returns the Syncthing device's system status.
func (s *Syncthing) SystemStatus() SystemStatus {
	return s.systemStatus
}

// SystemConnections Returns information about the Syncthing device's connections.
func (s *Syncthing) SystemConnections() SystemConnections {
	return s.systemConnections
}

// ConnectedDevices Returns a mapping from the Device ID to information about a connected Syncthing device.
func (s *Syncthing) ConnectedDevices() map[string]ConnectionStats {
	return s.systemConnections.Connections
}

// IntroducedDevices Provides a map of all of the devices which are introduced,
// of the format device.ID -> device.
func (s *Syncthing) IntroducedDevices() map[string]config.DeviceConfiguration {
	introduced := map[string]config.DeviceConfiguration{}
	for _, device := range s.configuration.Devices {
		if device.IntroducedBy.String() != "" {
			introduced[device.DeviceID.GoString()] = device
		}
	}
	return introduced
}

// jsonRequest Makes an HTTPS request to the API at the .
//nolint:funlen,lll,unparam,unused
func (s *Syncthing) jsonRequest(endpoint string, method string, requestBody interface{}) ([]byte, error) {
	// marshal above json body into a string
	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}
	// tostring the json body
	body := io.Reader(bytes.NewReader(jsonBody))

	// build new client if none exists
	req, err := http.NewRequest(method, s.apiConfig.APIURL+endpoint, body)
	if err != nil {
		return nil, err
	}

	// set headers
	headers, err := s.apiConfig.Headers()
	if err != nil {
		return nil, err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := s.apiConfig.Client.Do(req)
	if err != nil {
		return nil, err
	}

	// if there was an error, provide the information
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	// read body into response
	return ioutil.ReadAll(resp.Body)
}

// fetchConfig Fetches the latest configuration data from the Syncthing API
// and uses it to update the local Syncthing object.
func (s *Syncthing) fetchConfig() (*config.Configuration, error) {
	responseBody := &config.Configuration{}
	s.logger.Info("Fetching Syncthing config")
	data, err := s.jsonRequest("/rest/config", "GET", nil)
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(data, responseBody); err != nil {
		return nil, err
	}
	return responseBody, nil
}

// fetchSystemStatus Fetches the system status from the Syncthing API,
// and returns a SystemStatus object on success, or an error on failure.
func (s *Syncthing) fetchSystemStatus() (*SystemStatus, error) {
	responseBody := &SystemStatus{}
	s.logger.Info("Fetching Syncthing system status")
	data, err := s.jsonRequest("/rest/system/status", "GET", nil)
	if err != nil {
		return nil, err
	}
	// unmarshal the data into the responseBody
	err = json.Unmarshal(data, responseBody)
	if err != nil {
		return nil, err
	}
	return responseBody, nil
}

// fetchSystemConnections Fetches information regarding the connections with the running
// Syncthing node from Syncthing's API. Returns a SystemConnections object if successful, error otherwise.
func (s *Syncthing) fetchSystemConnections() (*SystemConnections, error) {
	// updates the connected status if successful, else returns an error
	responseBody := &SystemConnections{
		Connections: map[string]ConnectionStats{},
	}
	s.logger.Info("Fetching Syncthing connected status")
	data, err := s.jsonRequest("/rest/system/connections", "GET", nil)
	if err != nil {
		return nil, err
	}

	if err = json.Unmarshal(data, responseBody); err != nil {
		return nil, err
	}
	return responseBody, nil
}

// APIConfig Returns the current API Configuration
func (s *Syncthing) APIConfig() APIConfig {
	return s.apiConfig
}

// SetAPIConfig Overwrites Syncthing's API configuration with the provided object.
func (s *Syncthing) SetAPIConfig(apiConfig APIConfig) {
	s.apiConfig = apiConfig
}

// SetConfig Takes the given configuration object and uses it to update the local
// Syncthing object.
func (s *Syncthing) SetConfig(c config.Configuration) {
	s.configuration = c
}

// Fetch Pulls all of Syncthing's latest information from the API and stores it
// in the object's local storage.
func (s *Syncthing) Fetch() error {
	// get & store config
	conf, err := s.fetchConfig()
	if err != nil {
		return err
	}
	s.configuration = *conf

	// get & store connection info
	systemConnections, err := s.fetchSystemConnections()
	if err != nil {
		return err
	}
	s.systemConnections = *systemConnections

	// get and store system status
	systemStatus, err := s.fetchSystemStatus()
	if err != nil {
		return err
	}
	s.systemStatus = *systemStatus

	return nil
}

// PublishConfig Updates the Syncthing API with the stored configuration data.
// An error is returned in the case of a failure.
func (s *Syncthing) PublishConfig() error {
	// update the config
	s.logger.Info("Updating Syncthing config")
	_, err := s.jsonRequest("/rest/config", "PUT", s.configuration)
	if err != nil {
		s.logger.Error(err, "Failed to update Syncthing config")
	}
	return err
}

// NewSyncthingAPI Creates a new Syncthing object which implements the SyncthingAPI
// interface. This takes a logger as an input.
func NewSyncthingAPI(logger logr.Logger) SyncthingAPI {
	return &Syncthing{
		logger: logger,
	}
}

// checkResponse Returns an error if one exists in the response, or nil otherwise.
// This function was extracted from the Syncthing repository
// due to the overlapping functionality between our API access & the Syncthing CLI.
// nolint:lll
// see: https://github.com/syncthing/syncthing/blob/a162e8d9f926f3bcfb5deb7f9abbb960d40b1f5b/cmd/syncthing/cli/client.go#L158
func checkResponse(response *http.Response) error {
	if response.StatusCode == http.StatusNotFound {
		return errors.New("invalid endpoint or API call")
	} else if response.StatusCode == http.StatusUnauthorized {
		return errors.New("invalid API key")
	} else if response.StatusCode != http.StatusOK {
		data, err := responseToBArray(response)
		if err != nil {
			return err
		}
		body := strings.TrimSpace(string(data))
		return fmt.Errorf("unexpected HTTP status returned: %s\n%s", response.Status, body)
	}
	return nil
}

// responseToBArray Takes a given HTTP Response object and returns the body as a byte array.
// This function was extracted from the Syncthing repository
// due to the overlapping functionality between our API access & the Syncthing CLI.
// nolint:lll
// see: https://github.com/syncthing/syncthing/blob/a162e8d9f926f3bcfb5deb7f9abbb960d40b1f5b/cmd/syncthing/cli/utils.go#L25
func responseToBArray(response *http.Response) ([]byte, error) {
	bytes, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	return bytes, response.Body.Close()
}
