// Package config defines the minimal set of Syncthing configuration types
// needed for VolSync's REST API communication with the Syncthing binary.
// These types mirror the JSON structure of the Syncthing REST API responses
// (see https://docs.syncthing.net/rest/config.html) but only include fields
// that VolSync actually uses.
package config

type Configuration struct {
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
