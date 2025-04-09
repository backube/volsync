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
	"crypto/tls"
	"net/http"

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
	APIURL string `json:"apiURL"`
	APIKey string `json:"apiKey"`
	// don't marshal this field
	TLSConfig *tls.Config
	Client    *http.Client
}

type SyncthingConnection interface {
	// API Functions, these are meant to define communication with the Syncthing API.
	Fetch() (*Syncthing, error)
	PublishConfig(config.Configuration) error
}

// Syncthing Defines a Syncthing API object which contains a subset of the information
// exposed through Syncthing's API. Namely, this struct exposes the configuration,
// system status, and connections contained by the given object.
type Syncthing struct {
	Configuration     config.Configuration
	SystemConnections SystemConnections
	SystemStatus      SystemStatus
}
