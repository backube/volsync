package syncthing

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"

	"github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/controllers"
)

func (st *Syncthing) UpdateDevices(peerList []v1alpha1.SyncthingPeer) {
	st.logger.V(4).Info("Updating devices", "peerlist", peerList)

	// update syncthing config based on the provided peerlist
	newDevices := []SyncthingDevice{}
	// add myself to the device list
	for _, device := range st.Config.Devices {
		if device.DeviceID == st.SystemStatus.MyID {
			newDevices = append(newDevices, device)
			break
		}
	}

	for _, device := range peerList {
		// skip self
		if device.ID == st.SystemStatus.MyID {
			continue
		}
		stDeviceToAdd := SyncthingDevice{
			DeviceID:   device.ID,
			Addresses:  []string{device.Address},
			Name:       "Syncthing Device " + string(rune(len(newDevices))),
			Introducer: device.Introducer,
		}
		st.logger.V(4).Info("Adding device: %+v\n", stDeviceToAdd)
		newDevices = append(newDevices, stDeviceToAdd)
	}

	st.Config.Devices = newDevices
	st.logger.V(4).Info("Updated devices", "devices", st.Config.Devices)

	// update the folders
	st.UpdateFolders()
}

func (st *Syncthing) UpdateFolders() {
	// share the current folder(s) with the new devices
	var newFolders = []SyncthingFolder{}
	for _, folder := range st.Config.Folders {
		// copy folder & reset
		newFolder := folder
		newFolder.Devices = []FolderDeviceConfiguration{}
		for _, device := range st.Config.Devices {
			newFolder.Devices = append(newFolder.Devices, FolderDeviceConfiguration{
				DeviceID: device.DeviceID,
			})
		}
		newFolders = append(newFolders, newFolder)
	}
	st.Config.Folders = newFolders
}

func (st *Syncthing) NeedsReconfigure(nodeList []v1alpha1.SyncthingPeer) bool {
	// check if the syncthing nodelist diverges from the current syncthing devices
	var newDevices map[string]v1alpha1.SyncthingPeer = map[string]v1alpha1.SyncthingPeer{
		st.SystemStatus.MyID: {
			ID:      st.SystemStatus.MyID,
			Address: "",
		},
	}
	for _, device := range nodeList {
		newDevices[device.ID] = device
	}

	// create a map for current devices
	var currentDevs map[string]v1alpha1.SyncthingPeer = map[string]v1alpha1.SyncthingPeer{}
	for _, device := range st.Config.Devices {
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

func (st *Syncthing) UpdateSyncthingConfig() error {
	// update the config
	st.logger.V(4).Info("Updating Syncthing config")
	_, err := controllers.JSONRequest(st.APIConfig.APIURL+"/rest/config", "PUT", st.APIConfig.Headers(), st.Config)
	if err != nil {
		st.logger.V(4).Error(err, "Failed to update Syncthing config")
		return err
	}
	return err
}

func (st *Syncthing) FetchSyncthingConfig() error {
	responseBody := &SyncthingConfig{
		Devices: []SyncthingDevice{},
		Folders: []SyncthingFolder{},
	}
	st.logger.V(4).Info("Fetching Syncthing config")
	data, err := controllers.JSONRequest(st.APIConfig.APIURL+"/rest/config", "GET", st.APIConfig.Headers(), nil)
	if err != nil {
		return err
	}
	err = json.Unmarshal(data, responseBody)
	st.Config = responseBody
	return err
}

func (st *Syncthing) FetchSyncthingSystemStatus() error {
	responseBody := &SystemStatus{}
	st.logger.V(4).Info("Fetching Syncthing system status")
	data, err := controllers.JSONRequest(st.APIConfig.APIURL+"/rest/system/status", "GET", st.APIConfig.Headers(), nil)
	if err != nil {
		return err
	}
	// unmarshal the data into the responseBody
	err = json.Unmarshal(data, responseBody)
	st.SystemStatus = responseBody
	return err
}

func (st *Syncthing) FetchConnectedStatus() error {
	// updates the connected status if successful, else returns an error
	responseBody := &SystemConnections{
		Connections: map[string]ConnectionStats{},
	}
	st.logger.V(4).Info("Fetching Syncthing connected status")
	data, err := controllers.JSONRequest(
		st.APIConfig.APIURL+"/rest/system/connections", "GET", st.APIConfig.Headers(), nil,
	)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(data, responseBody); err == nil {
		st.SystemConnections = responseBody
	}
	return err
}

func (api *APIConfig) Headers() map[string]string {
	return map[string]string{
		"X-API-Key":    api.APIKey,
		"Content-Type": "application/json",
	}
}

func GenerateRandomBytes(length int) ([]byte, error) {
	// generates random bytes of given length
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func GenerateRandomString(length int) (string, error) {
	// generate a random string
	b, err := GenerateRandomBytes(length)
	return base64.URLEncoding.EncodeToString(b), err
}
