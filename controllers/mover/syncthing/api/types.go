package api

import (
	"crypto/tls"
	"net/http"

	"github.com/go-logr/logr"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/connections"
)

// DialStatus Provides us with information as to whether or not we are able to
// dial a given device, when the last time we dialed was, and what the error is, if any.
type DialStatus struct {
	When  string  `json:"when"`
	Error *string `json:"error"`
	OK    bool    `json:"ok"`
}

// SystemStatus Details information about the running Syncthing system, including the
// device ID, CPU usage, allocated memory, number of goroutines, when it started, and so on and so forth.
type SystemStatus struct {
	Alloc                   int                                        `json:"alloc"`
	ConnectionServiceStatus map[string]connections.ListenerStatusEntry `json:"connectionServiceStatus"`
	CPUPercent              int                                        `json:"cpuPercent"`
	Goroutines              int                                        `json:"goroutines"`
	GUIAddressOverridden    bool                                       `json:"guiAddressOverridden"`
	GUIAddressUsed          string                                     `json:"guiAddressUsed"`
	LastDialStatus          map[string]DialStatus                      `json:"lastDialStatus"`
	MyID                    string                                     `json:"myID"`
	PathSeparator           string                                     `json:"pathSeparator"`
	StartTime               string                                     `json:"startTime"`
	Sys                     int                                        `json:"sys"`
	Tilde                   string                                     `json:"tilde"`
	Uptime                  int                                        `json:"uptime"`
	URVersionMax            int                                        `json:"urVersionMax"`
}

// TotalStats Describes the total traffic to/from a given Syncthing node.
type TotalStats struct {
	At            string `json:"at"`
	InBytesTotal  int    `json:"inBytesTotal"`
	OutBytesTotal int    `json:"outBytesTotal"`
}

// ConnectionStats Details statistics about this Syncthing connection.
type ConnectionStats struct {
	TotalStats
	Connected     bool   `json:"connected"`
	Paused        bool   `json:"paused"`
	At            string `json:"at"`
	StartedAt     string `json:"startedAt"`
	ClientVersion string `json:"clientVersion"`
	Address       string `json:"address"`
	Type          string `json:"type"`
}

// SystemConnections Describes the devices which are connected to the Syncthing
// device, in addition to statistics about the total traffic to and from this node.
type SystemConnections struct {
	Total       TotalStats                 `json:"total"`
	Connections map[string]ConnectionStats `json:"connections"`
}

// APIConfig Describes the necessary elements needed to configure a client
// with the Syncthing API, included the credentials, URL, TLS Certs.
// This requires nolint:revive because the package it's in is called "api,"
// and it's meant to be used in an interface which already contains `Config`
// meaning a different thing.
// nolint:revive
type APIConfig struct {
	APIURL      string `json:"apiURL"`
	APIKey      string `json:"apiKey"`
	GUIUser     string `json:"user"`
	GUIPassword string `json:"password"`
	// don't marshal this field
	TLSConfig *tls.Config
	Client    *http.Client
}

// Syncthing Defines a Syncthing API object which contains a subset of the information
// exposed through Syncthing's API. Namely, this struct exposes the configuration,
// system status, and connections contained by the given object.
type Syncthing struct {
	configuration     config.Configuration
	systemConnections SystemConnections
	systemStatus      SystemStatus
	apiConfig         APIConfig
	logger            logr.Logger
}

// Syncthing Exposes an interface for structs to implement the Syncthing API
// and provide important data while abstracting the specifics of the Syncthing API.
type SyncthingAPI interface {
	// Data Functions, meant for storage and retrieval of local object data
	Config() config.Configuration
	SetConfig(config.Configuration)

	// Immutable accessors
	SystemStatus() SystemStatus
	SystemConnections() SystemConnections

	// Specific accessor methods for convenience
	ConnectedDevices() map[string]ConnectionStats
	MyID() string

	// Derivative accessor
	IntroducedDevices() map[string]config.DeviceConfiguration

	// Getters & Setters for API Config
	APIConfig() APIConfig
	SetAPIConfig(APIConfig)

	// API Functions, these are meant to define communication with the Syncthing API.
	Fetch() error
	PublishConfig() error

	// private methods
	fetchConfig() (*config.Configuration, error)
	fetchSystemConnections() (*SystemConnections, error)
	fetchSystemStatus() (*SystemStatus, error)
	jsonRequest(endpoint string, method string, requestBody interface{}) ([]byte, error)
}
