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
package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-logr/logr"
	"github.com/syncthing/syncthing/lib/config"
)

// syncthingAPIConnection Is an API Connection struct which implements the SyncthingConnection
// interface.
type syncthingAPIConnection struct {
	apiConfig APIConfig
	logger    logr.Logger
}

// headers Returns a map containing the necessary headers for Syncthing API requests.
// When no API Key is provided, an error is returned.
func (api *syncthingAPIConnection) headers() map[string]string {
	return map[string]string{
		"X-API-Key":    api.apiConfig.APIKey,
		"Content-Type": "application/json",
	}
}

// jsonRequest Makes an HTTPS request to the API at the .
func (api *syncthingAPIConnection) jsonRequest(
	endpoint string,
	method string,
	requestBody interface{},
) ([]byte, error) {
	// marshal above json body into a string
	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}
	// tostring the json body
	body := io.Reader(bytes.NewReader(jsonBody))

	// build new client if none exists
	req, err := http.NewRequest(method, api.apiConfig.APIURL+endpoint, body)
	if err != nil {
		return nil, err
	}

	// set headers
	headers := api.headers()
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := api.apiConfig.Client.Do(req)
	if err != nil {
		return nil, err
	}

	// if there was an error, provide the information
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	// read body into response
	return io.ReadAll(resp.Body)
}

// fetchConfig Fetches the latest configuration data from the Syncthing API
// and uses it to update the local Syncthing object.
func (api *syncthingAPIConnection) fetchConfig() (*config.Configuration, error) {
	responseBody := &config.Configuration{}
	api.logger.Info("Fetching Syncthing config")
	data, err := api.jsonRequest(ConfigEndpoint, "GET", nil)
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
func (api *syncthingAPIConnection) fetchSystemStatus() (*SystemStatus, error) {
	responseBody := &SystemStatus{}
	api.logger.Info("Fetching Syncthing system status")
	data, err := api.jsonRequest(SystemStatusEndpoint, "GET", nil)
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
func (api *syncthingAPIConnection) fetchSystemConnections() (*SystemConnections, error) {
	// updates the connected status if successful, else returns an error
	responseBody := &SystemConnections{
		Connections: map[string]ConnectionStats{},
	}
	api.logger.Info("Fetching Syncthing connected status")
	data, err := api.jsonRequest(SystemConnectionsEndpoint, "GET", nil)
	if err != nil {
		return nil, err
	}

	if err = json.Unmarshal(data, responseBody); err != nil {
		return nil, err
	}
	return responseBody, nil
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
