package syncthing

import (
	"encoding/json"

	"github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/controllers"
)

/**************************** EXPORTED FUNCTIONS *******************************/

func (st *Syncthing) UpdateDevices(peerList []v1alpha1.SyncthingPeer) {
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
		if device.ID == st.SystemStatus.MyID {
			continue
		}
		newDevices = append(newDevices, SyncthingDevice{
			DeviceID:   device.ID,
			Addresses:  []string{device.Address},
			Name:       "Syncthing Device " + string(rune(len(newDevices))),
			Introducer: device.Introducer,
		})
	}
	st.Config.Devices = newDevices
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
	if err := st.fetchSyncthingConfig(); err != nil {
		return err
	}
	if err := st.fetchSyncthingSystemStatus(); err != nil {
		return err
	}
	if err := st.fetchConnectedStatus(); err != nil {
		return err
	}
	return nil
}

func (st *Syncthing) UpdateSyncthingConfig() error {
	// update the config
	_, err := controllers.JSONRequest(st.APIConfig.APIURL+"/rest/config", "PUT", st.APIConfig.Headers(), st.Config)
	if err != nil {
		return err
	}
	return err
}

/**************************** INTERNAL FUNCTIONS *******************************/

func (st *Syncthing) fetchSyncthingConfig() error {
	responseBody := &SyncthingConfig{
		Devices: []SyncthingDevice{},
		Folders: []SyncthingFolder{},
	}
	data, err := controllers.JSONRequest(st.APIConfig.APIURL+"/rest/config", "GET", st.APIConfig.Headers(), nil)
	if err != nil {
		return err
	}
	err = json.Unmarshal(data, responseBody)
	st.Config = responseBody
	return err
}

func (st *Syncthing) fetchSyncthingSystemStatus() error {
	responseBody := &SystemStatus{}
	data, err := controllers.JSONRequest(st.APIConfig.APIURL+"/rest/system/status", "GET", st.APIConfig.Headers(), nil)
	if err != nil {
		return err
	}
	// unmarshal the data into the responseBody
	err = json.Unmarshal(data, responseBody)
	st.SystemStatus = responseBody
	return err
}

func (st *Syncthing) fetchConnectedStatus() error {
	// updates the connected status if successful, else returns an error
	responseBody := &SystemConnections{
		Connections: map[string]ConnectionStats{},
	}
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
