// Package config defines the minimal set of Syncthing configuration types
// needed for VolSync's REST API communication with the Syncthing binary.
// These types mirror the JSON structure of the Syncthing REST API responses
// (see https://docs.syncthing.net/rest/config.html) but only include fields
// that VolSync actually uses.
package config

type Configuration struct {
	Version int                   `json:"version"`
	Folders []FolderConfiguration `json:"folders"`
	Devices []DeviceConfiguration `json:"devices"`
	GUI     GUIConfiguration      `json:"gui"`
}

type DeviceConfiguration struct {
	DeviceID     string   `json:"deviceID"`
	Name         string   `json:"name,omitempty"`
	Addresses    []string `json:"addresses,omitempty"`
	Introducer   bool     `json:"introducer"`
	IntroducedBy string   `json:"introducedBy"`
}

type FolderConfiguration struct {
	ID      string                      `json:"id"`
	Label   string                      `json:"label"`
	Path    string                      `json:"path"`
	Devices []FolderDeviceConfiguration `json:"devices"`
}

type FolderDeviceConfiguration struct {
	DeviceID     string `json:"deviceID"`
	IntroducedBy string `json:"introducedBy"`
}

type GUIConfiguration struct {
	RawAddress string `json:"address,omitempty"`
	User       string `json:"user,omitempty"`
	Password   string `json:"password,omitempty"`
}

// SetDevice adds a device to the configuration if it doesn't exist,
// or updates it if it does.
func (cfg *Configuration) SetDevice(device DeviceConfiguration) {
	for i, d := range cfg.Devices {
		if d.DeviceID == device.DeviceID {
			cfg.Devices[i] = device
			return
		}
	}
	cfg.Devices = append(cfg.Devices, device)
}

// SetDevices replaces the entire device list.
func (cfg *Configuration) SetDevices(devices []DeviceConfiguration) {
	cfg.Devices = devices
}

// DeviceMap returns a map of device ID to device configuration.
func (cfg *Configuration) DeviceMap() map[string]DeviceConfiguration {
	m := make(map[string]DeviceConfiguration, len(cfg.Devices))
	for _, d := range cfg.Devices {
		m[d.DeviceID] = d
	}
	return m
}

// SetFolder adds a folder to the configuration if it doesn't exist,
// or updates it if it does.
func (cfg *Configuration) SetFolder(folder FolderConfiguration) {
	for i, f := range cfg.Folders {
		if f.ID == folder.ID {
			cfg.Folders[i] = folder
			return
		}
	}
	cfg.Folders = append(cfg.Folders, folder)
}

// SetFolders replaces the entire folder list.
func (cfg *Configuration) SetFolders(folders []FolderConfiguration) {
	cfg.Folders = folders
}
