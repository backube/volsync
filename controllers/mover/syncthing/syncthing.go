package syncthing

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/backube/volsync/api/v1alpha1"
)

// UpdateDevices Updates the Syncthing's connected devices with the provided peerList.
func (st *Syncthing) UpdateDevices(peerList []v1alpha1.SyncthingPeer) {
	st.logger.V(4).Info("Updating devices", "peerlist", peerList)

	// update syncthing config based on the provided peerlist
	newDevices := []SyncthingDevice{}

	// add myself and introduced devices to the device list
	for _, device := range st.Config.Devices {
		if device.DeviceID == st.SystemStatus.MyID || device.IntroducedBy != "" {
			newDevices = append(newDevices, device)
		}
	}

	// Add the devices from the peerList to the device list
	for _, device := range peerList {
		stDeviceToAdd := SyncthingDevice{
			DeviceID:   device.ID,
			Addresses:  []string{device.Address},
			Introducer: device.Introducer,
		}
		st.logger.V(4).Info("Adding device: %+v\n", stDeviceToAdd)
		newDevices = append(newDevices, stDeviceToAdd)
	}

	st.Config.Devices = newDevices
	st.logger.V(4).Info("Updated devices", "devices", st.Config.Devices)

	// update folders with the new devices
	st.updateFolders()
}

// updateFolders Updates all of Syncthing's folders to be shared with all configured devices.
func (st *Syncthing) updateFolders() {
	// share the current folder(s) with the new devices
	var newFolders = []SyncthingFolder{}
	for _, folder := range st.Config.Folders {
		// copy folder & reset
		newFolder := folder
		newFolder.Devices = []FolderDeviceConfiguration{}

		for _, device := range st.Config.Devices {
			newFolder.Devices = append(newFolder.Devices, FolderDeviceConfiguration{
				DeviceID:     device.DeviceID,
				IntroducedBy: device.IntroducedBy,
			})
		}
		newFolders = append(newFolders, newFolder)
	}
	st.Config.Folders = newFolders
}

// NeedsReconfigure Determines whether the given nodeList differs from Syncthing's internal devices.
func (st *Syncthing) NeedsReconfigure(nodeList []v1alpha1.SyncthingPeer) bool {
	// check if the syncthing nodelist diverges from the current syncthing devices
	var newDevices map[string]v1alpha1.SyncthingPeer = map[string]v1alpha1.SyncthingPeer{
		// initialize the map with the self node
		st.SystemStatus.MyID: {
			ID:      st.SystemStatus.MyID,
			Address: "",
		},
	}

	// add all of the other devices in the provided nodeList
	for _, device := range nodeList {
		// avoid self
		if device.ID == st.SystemStatus.MyID {
			continue
		}
		newDevices[device.ID] = device
	}

	// create a map for current devices
	var currentDevs map[string]v1alpha1.SyncthingPeer = map[string]v1alpha1.SyncthingPeer{
		// initialize the map with the self node
		st.SystemStatus.MyID: {
			ID:      st.SystemStatus.MyID,
			Address: "",
		},
	}
	// add the rest of devices to the map
	for _, device := range st.Config.Devices {
		// ignore self and introduced devices
		if device.DeviceID == st.SystemStatus.MyID || device.IntroducedBy != "" {
			continue
		}

		currentDevs[device.DeviceID] = v1alpha1.SyncthingPeer{
			ID:      device.DeviceID,
			Address: device.Addresses[0],
		}
	}

	// check if the syncthing nodelist diverges from the current syncthing devices
	for _, device := range newDevices {
		if _, ok := currentDevs[device.ID]; !ok {
			return true
		}
	}
	for _, device := range currentDevs {
		if _, ok := newDevices[device.ID]; !ok {
			return true
		}
	}
	return false
}

// collectIntroduced Returns a map of DeviceID -> Device for devices which have been introduced to us by another node.
func (st *Syncthing) collectIntroduced() map[string]SyncthingDevice {
	introduced := map[string]SyncthingDevice{}
	for _, device := range st.Config.Devices {
		if device.IntroducedBy != "" {
			introduced[device.DeviceID] = device
		}
	}
	return introduced
}

// PeerListContainsIntroduced Returns 'true' if the given peerList contains a node
// which has been introduced to us by another Syncthing instance, 'false' otherwise.
func (st *Syncthing) PeerListContainsIntroduced(peerList []v1alpha1.SyncthingPeer) bool {
	introducedSet := st.collectIntroduced()

	// check if the peerList contains an introduced node
	for _, peer := range peerList {
		if _, ok := introducedSet[peer.ID]; ok {
			return true
		}
	}
	return false
}

// PeerListContainsSelf Returns 'true' if the given peerList contains the self node, 'false' otherwise.
func (st *Syncthing) PeerListContainsSelf(peerList []v1alpha1.SyncthingPeer) bool {
	for _, peer := range peerList {
		if peer.ID == st.SystemStatus.MyID {
			return true
		}
	}
	return false
}

// GetDeviceFromID Returns the device with the given ID,
// along with a boolean indicating whether the device was found.
func (st *Syncthing) GetDeviceFromID(deviceID string) (SyncthingDevice, bool) {
	for _, device := range st.Config.Devices {
		if device.DeviceID == deviceID {
			return device, true
		}
	}
	return SyncthingDevice{}, false
}

// FetchLatestInfo Updates the Syncthing object with the latest data fetched from the Syncthing API.
func (st *Syncthing) FetchLatestInfo() error {
	if err := st.FetchSyncthingConfig(); err != nil {
		return err
	}
	if err := st.FetchSyncthingSystemStatus(); err != nil {
		return err
	}
	if err := st.FetchConnectedStatus(); err != nil {
		return err
	}
	return nil
}

// UpdateSyncthingConfig Updates the Syncthing config with the locally-stored config.
func (st *Syncthing) UpdateSyncthingConfig() error {
	// update the config
	st.logger.V(4).Info("Updating Syncthing config")
	_, err := st.jsonRequest("/rest/config", "PUT", st.Config)
	if err != nil {
		st.logger.V(4).Error(err, "Failed to update Syncthing config")
		return err
	}
	return err
}

// FetchSyncthingConfig Fetches the latest configuration data from the Syncthing API
// and uses it to update the local Syncthing object.
func (st *Syncthing) FetchSyncthingConfig() error {
	responseBody := &SyncthingConfig{
		Devices: []SyncthingDevice{},
		Folders: []SyncthingFolder{},
	}
	st.logger.V(4).Info("Fetching Syncthing config")
	data, err := st.jsonRequest("/rest/config", "GET", nil)
	if err != nil {
		return err
	}
	err = json.Unmarshal(data, responseBody)
	st.Config = responseBody
	return err
}

// FetchSyncthingSystemStatus fetches the Syncthing system status.
func (st *Syncthing) FetchSyncthingSystemStatus() error {
	responseBody := &SystemStatus{}
	st.logger.V(4).Info("Fetching Syncthing system status")
	data, err := st.jsonRequest("/rest/system/status", "GET", nil)
	if err != nil {
		return err
	}
	// unmarshal the data into the responseBody
	err = json.Unmarshal(data, responseBody)
	st.SystemStatus = responseBody
	return err
}

// FetchConnectedStatus Fetches the connection status of the syncthing instance.
func (st *Syncthing) FetchConnectedStatus() error {
	// updates the connected status if successful, else returns an error
	responseBody := &SystemConnections{
		Connections: map[string]ConnectionStats{},
	}
	st.logger.V(4).Info("Fetching Syncthing connected status")
	data, err := st.jsonRequest("/rest/system/connections", "GET", nil)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(data, responseBody); err == nil {
		st.SystemConnections = responseBody
	}
	return err
}

// jsonRequest performs a request to the Syncthing API and returns the response body.
//nolint:funlen,lll,unparam,unused
func (st *Syncthing) jsonRequest(endpoint string, method string, requestBody interface{}) ([]byte, error) {
	// marshal above json body into a string
	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}
	// tostring the json body
	body := io.Reader(bytes.NewReader(jsonBody))

	// build new client if none exists
	req, err := http.NewRequest(method, st.APIConfig.APIURL+endpoint, body)
	if err != nil {
		return nil, err
	}

	// set headers
	headers, err := st.APIConfig.Headers()
	if err != nil {
		return nil, err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := st.APIConfig.Client.Do(req)
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

// Headers Returns a map containing the necessary headers for Syncthing API requests.
// When no API Key is provided, an error is returned.
func (api *APIConfig) Headers() (map[string]string, error) {
	if api.APIKey == "" {
		return nil, errors.New("API Key is not set")
	}

	return map[string]string{
		"X-API-Key":    api.APIKey,
		"Content-Type": "application/json",
	}, nil
}

// BuildTLSClient Returns a new TLS client for Syncthing API requests.
func (api *APIConfig) BuildOrUseExistingTLSClient() *http.Client {
	if api.Client != nil {
		return api.Client
	}
	return api.BuildTLSClient()
}

// BuildTLSClient Returns a new TLS client for Syncthing API requests.
func (api *APIConfig) BuildTLSClient() *http.Client {
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

// GenerateRandomBytes Generates random bytes of the given length using the OS's RNG.
func GenerateRandomBytes(length int) ([]byte, error) {
	// generates random bytes of given length
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// GenerateRandomString Generates a random string of ASCII characters excluding control characters
// 0-31, 32 (space), and 127.
// the given length using the OS's RNG.
func GenerateRandomString(length int) (string, error) {
	// generate a random string
	b, err := GenerateRandomBytes(length)
	if err != nil {
		return "", err
	}

	// construct string by mapping the randomly generated bytes into
	// a range of acceptable characters
	var lowerBound byte = 33
	var upperBound byte = 126
	var acceptableRange byte = upperBound - lowerBound + 1

	// generate the string by mapping [0, 255] -> [33, 126]
	var acceptableBytes []byte = []byte{}
	for i := 0; i < len(b); i++ {
		// normalize number to be in the range [33, 126] inclusive
		acceptableByte := (b[i] % acceptableRange) + lowerBound
		acceptableBytes = append(acceptableBytes, acceptableByte)
	}
	return string(acceptableBytes), nil
}

// asTCPAddress Accepts a partial URL which may be a hostname or a hostname:port, and returns a TCP address.
// If the provided address already contains a prefix, then it is
func asTCPAddress(addr string) string {
	// check if TCP is already prefixed
	if strings.HasPrefix(addr, "tcp://") {
		return addr
	}
	return "tcp://" + addr
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
