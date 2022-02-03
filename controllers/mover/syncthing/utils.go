package syncthing

import (
	"github.com/backube/volsync/api/v1alpha1"
)

func UpdateDevices(m *Mover, config *SyncthingConfig, status *SystemStatus) []SyncthingDevice {
	newDevices := []SyncthingDevice{}
	for _, device := range config.Devices {
		if device.DeviceID == status.MyID {
			newDevices = append(newDevices, device)
			break
		}
	}

	for _, device := range m.peerList {
		if device.ID == status.MyID {
			continue
		}
		newDevices = append(newDevices, SyncthingDevice{
			DeviceID:   device.ID,
			Addresses:  []string{device.Address},
			Name:       "Syncthing Device " + string(rune(len(newDevices))),
			Introducer: device.Introducer,
		})
	}
	return newDevices
}

func UpdateFolders(config *SyncthingConfig) []SyncthingFolder {
	// share the current folder(s) with the new devices
	var newFolders = []SyncthingFolder{}
	for _, folder := range config.Folders {
		for _, device := range config.Devices {
			folder.Devices = append(folder.Devices, FolderDeviceConfiguration{
				DeviceID: device.DeviceID,
			})
		}
		newFolders = append(newFolders, folder)
	}
	return newFolders
}

func NeedsReconfigure(connectedDevs []SyncthingDevice, nodeList []v1alpha1.SyncthingPeer, selfID string) bool {
	// check if the syncthing nodelist diverges from the current syncthing devices
	var newDevices map[string]v1alpha1.SyncthingPeer = map[string]v1alpha1.SyncthingPeer{
		selfID: {
			ID:      selfID,
			Address: "",
		},
	}
	for _, device := range nodeList {
		newDevices[device.ID] = device
	}

	// create a map for current devices
	var currentDevs map[string]v1alpha1.SyncthingPeer = map[string]v1alpha1.SyncthingPeer{}
	for _, device := range connectedDevs {
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
