//nolint:revive
/*
Copyright 2021 The VolSync authors.

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

package syncthing

// syncthing config type
// nolint:revive
type SyncthingDevice struct {
	DeviceID                 string   `json:"deviceID"`
	Name                     string   `json:"name"`
	Addresses                []string `json:"addresses"`
	Compression              string   `json:"compression"`
	CertName                 string   `json:"certName"`
	Introducer               bool     `json:"introducer"`
	SkipIntroductionRemovals bool     `json:"skipIntroductionRemovals"`
	IntroducedBy             string   `json:"introducedBy"`
	Paused                   bool     `json:"paused"`
	AllowedNetworks          []string `json:"allowedNetworks"`
	AutoAcceptFolders        bool     `json:"autoAcceptFolders"`
	MaxSendKbps              int      `json:"maxSendKbps"`
	MaxRecvKbps              int      `json:"maxRecvKbps"`
	IgnoredFolders           []string `json:"ignoredFolders"`
	MaxRequestKiB            int      `json:"maxRequestKiB"`
	Untrusted                bool     `json:"untrusted"`
	RemoteGUIPort            int      `json:"remoteGUIPort"`
}

//nolint:revive
type SyncthingSize struct {
	Value int    `json:"value"`
	Unit  string `json:"unit"`
}

//nolint:revive
type SyncthingVersioning struct {
	Type             string            `json:"type"`
	Params           map[string]string `json:"params"`
	CleanupIntervalS int               `json:"cleanupIntervalS"`
	FsPath           string            `json:"fsPath"`
	FsType           string            `json:"fsType"`
}

//nolint:revive
type SyncthingFolder struct {
	ID                    string                      `json:"id"`
	Label                 string                      `json:"label"`
	FilesystemType        string                      `json:"filesystemType"`
	Path                  string                      `json:"path"`
	Type                  string                      `json:"type"`
	Devices               []FolderDeviceConfiguration `json:"devices"`
	RescanIntervalS       int                         `json:"rescanIntervalS"`
	FsWatcherEnabled      bool                        `json:"fsWatcherEnabled"`
	FsWatcherDelayS       int                         `json:"fsWatcherDelayS"`
	IgnorePerms           bool                        `json:"ignorePerms"`
	AutoNormalize         bool                        `json:"autoNormalize"`
	MinDiskFree           SyncthingSize               `json:"minDiskFree"`
	Versioning            SyncthingVersioning         `json:"versioning"`
	Copiers               int                         `json:"copiers"`
	PullerMaxPendingKiB   int                         `json:"pullerMaxPendingKiB"`
	Hashers               int                         `json:"hashers"`
	Order                 string                      `json:"order"`
	IgnoreDelete          bool                        `json:"ignoreDelete"`
	ScanProgressIntervalS int                         `json:"scanProgressIntervalS"`
}

//nolint:revive
type SyncthingOptions struct {
	ListenAddresses         []string      `json:"listenAddresses"`
	GlobalAnnServers        []string      `json:"globalAnnServers"`
	GlobalAnnEnabled        bool          `json:"globalAnnEnabled"`
	NATEnabled              bool          `json:"natEnabled"`
	NATLeaseMinutes         int           `json:"natLeaseMinutes"`
	NATRenewalMinutes       int           `json:"natRenewalMinutes"`
	NATTimeout              int           `json:"natTimeout"`
	URAccepted              int           `json:"urAccepted"`
	URUniqueID              string        `json:"urUniqueID"`
	URDeclined              int           `json:"urDeclined"`
	URPaused                int           `json:"urPaused"`
	RestartOnWakeup         bool          `json:"restartOnWakeup"`
	AutoUpgradeIntervalH    int           `json:"autoUpgradeIntervalH"`
	KeepTemporariesH        int           `json:"keepTemporariesH"`
	CacheIgnoredFiles       bool          `json:"cacheIgnoredFiles"`
	ProgressUpdateIntervalS int           `json:"progressUpdateIntervalS"`
	SymlinksEnabled         bool          `json:"symlinksEnabled"`
	LimitBandwidthInLan     bool          `json:"limitBandwidthInLan"`
	MinHomeDiskFree         SyncthingSize `json:"minHomeDiskFree"`
	URURL                   string        `json:"urURL"`
	URInitialDelayS         int           `json:"urInitialDelayS"`
	URPostInsecurely        bool          `json:"urPostInsecurely"`
	URInitialTimeoutS       int           `json:"urInitialTimeoutS"`
	URReconnectIntervalS    int           `json:"urReconnectIntervalS"`
	OverwriteRemoteDevice   bool          `json:"overwriteRemoteDevice"`
	TempIndexMinBlocks      int           `json:"tempIndexMinBlocks"`
	UnackedNotificationIDs  []string      `json:"unackedNotificationIDs"`
	DefaultFolderIgnores    []string      `json:"defaultFolderIgnores"`
	DefaultFolderExcludes   []string      `json:"defaultFolderExcludes"`
	DefaultFolderIncludes   []string      `json:"defaultFolderIncludes"`
	OverwriteRemoteTimeoutS int           `json:"overwriteRemoteTimeoutS"`
	OverwriteRemoteIgnores  bool          `json:"overwriteRemoteIgnores"`
	OverwriteRemoteExcludes bool          `json:"overwriteRemoteExcludes"`
	OverwriteRemoteIncludes bool          `json:"overwriteRemoteIncludes"`
	DefaultReadOnly         bool          `json:"defaultReadOnly"`
	IgnoredFiles            []string      `json:"ignoredFiles"`
	MaxConflicts            int           `json:"maxConflicts"`
	MaxKnownDeleted         int           `json:"maxKnownDeleted"`
	MaxChangeKiB            int           `json:"maxChangeKiB"`
	MaxSendKiB              int           `json:"maxSendKiB"`
	MaxRecvKiB              int           `json:"maxRecvKiB"`
	MaxRequestKiB           int           `json:"maxRequestKiB"`
	ReconnectIntervalS      int           `json:"reconnectIntervalS"`
	ReconnectFailureCount   int           `json:"reconnectFailureCount"`
	ReconnectBackoffMinS    int           `json:"reconnectBackoffMinS"`
	ReconnectBackoffMaxS    int           `json:"reconnectBackoffMaxS"`
	ReconnectBackoffMaxExp  int           `json:"reconnectBackoffMaxExp"`
	ReconnectBackoffJitter  bool          `json:"reconnectBackoffJitter"`
	ReconnectBackoffMult    int           `json:"reconnectBackoffMult"`
	ReconnectBackoffExp     int           `json:"reconnectBackoffExp"`
	ReconnectBackoffFloor   int           `json:"reconnectBackoffFloor"`
	ReconnectBackoffCeiling int           `json:"reconnectBackoffCeiling"`
	ReconnectBackoffMax     int           `json:"reconnectBackoffMax"`
	ReconnectBackoffMin     int           `json:"reconnectBackoffMin"`
}

type FolderDeviceConfiguration struct {
	DeviceID           string `json:"deviceID"`
	IntroducedBy       string `json:"introducedBy"`
	EncryptionPassword string `json:"encryptionPassword"`
}

//nolint:revive
type SyncthingConfig struct {
	Version              int               `json:"version"`
	Folders              []SyncthingFolder `json:"folders"`
	Devices              []SyncthingDevice `json:"devices"`
	Options              SyncthingOptions  `json:"options"`
	Device               SyncthingDevice   `json:"device"`
	RemoteIgnoredDevices []string          `json:"remoteIgnoredDevices"`
	Defaults             SyncthingFolder   `json:"defaults"`
}

/*******************************************************************************
 * Syncthing System Status
 * Example of a response from GET /rest/system/status:
 {
  "alloc": 21476992,
  "connectionServiceStatus": {
    "quic://0.0.0.0:22000": {
      "error": null,
      "lanAddresses": [
        "quic://0.0.0.0:22000",
        "quic://10.244.0.14:22000"
      ],
      "wanAddresses": [
        "quic://0.0.0.0:22000",
        "quic://108.20.160.75:22000"
      ]
    },
    "tcp://0.0.0.0:22000": {
      "error": null,
      "lanAddresses": [
        "tcp://0.0.0.0:22000",
        "tcp://10.244.0.14:22000"
      ],
      "wanAddresses": [
        "tcp://0.0.0.0:0",
        "tcp://0.0.0.0:22000"
      ]
    }
  },
  "cpuPercent": 0,
  "goroutines": 59,
  "guiAddressOverridden": false,
  "guiAddressUsed": "[::]:8384",
  "lastDialStatus": {},
  "myID": "7NT7QDT-4ZHKSOP-MYCPM6L-NWUMDSS-7QY2Z5E-GTNIG4S-UE6NLM2-G4WYTA3",
  "pathSeparator": "/",
  "startTime": "2022-01-13T21:09:39Z",
  "sys": 40988696,
  "tilde": "/root",
  "uptime": 69126,
  "urVersionMax": 3
}
 ******************************************************************************/

type ListenerStatusEntry struct {
	Error        *string  `json:"error"`
	LANAddresses []string `json:"lanAddresses"`
	WANAddresses []string `json:"wanAddresses"`
}

type DialStatus struct {
	When  string  `json:"when"`
	Error *string `json:"error"`
	OK    bool    `json:"ok"`
}

type SystemStatus struct {
	Alloc                   int                            `json:"alloc"`
	ConnectionServiceStatus map[string]ListenerStatusEntry `json:"connectionServiceStatus"`
	CPUPercent              int                            `json:"cpuPercent"`
	Goroutines              int                            `json:"goroutines"`
	GUIAddressOverridden    bool                           `json:"guiAddressOverridden"`
	GUIAddressUsed          string                         `json:"guiAddressUsed"`
	LastDialStatus          map[string]DialStatus          `json:"lastDialStatus"`
	MyID                    string                         `json:"myID"`
	PathSeparator           string                         `json:"pathSeparator"`
	StartTime               string                         `json:"startTime"`
	Sys                     int                            `json:"sys"`
	Tilde                   string                         `json:"tilde"`
	Uptime                  int                            `json:"uptime"`
	URVersionMax            int                            `json:"urVersionMax"`
}

/*
Example of a system connections response:

{
   "total" : {
          "at" : "2015-11-07T17:29:47.691637262+01:00",
          "inBytesTotal" : 1479,
          "outBytesTotal" : 1318,
   },
   "connections" : {
          "YZJBJFX-RDBL7WY-6ZGKJ2D-4MJB4E7-ZATSDUY-LD6Y3L3-MLFUYWE-AEMXJAC" : {
             "connected" : true,
             "inBytesTotal" : 556,
             "paused" : false,
             "at" : "2015-11-07T17:29:47.691548971+01:00",
             "startedAt" : "2015-11-07T00:09:47Z",
             "clientVersion" : "v0.12.1",
             "address" : "127.0.0.1:22002",
             "type" : "TCP (Client)",
             "outBytesTotal" : 550
          },
          "DOVII4U-SQEEESM-VZ2CVTC-CJM4YN5-QNV7DCU-5U3ASRL-YVFG6TH-W5DV5AA" : {
             "outBytesTotal" : 0,
             "type" : "",
             "address" : "",
             "at" : "0001-01-01T00:00:00Z",
             "startedAt" : "0001-01-01T00:00:00Z",
             "clientVersion" : "",
             "paused" : false,
             "inBytesTotal" : 0,
             "connected" : false
          },
          "UYGDMA4-TPHOFO5-2VQYDCC-7CWX7XW-INZINQT-LE4B42N-4JUZTSM-IWCSXA4" : {
             "address" : "",
             "type" : "",
             "outBytesTotal" : 0,
             "connected" : false,
             "inBytesTotal" : 0,
             "paused" : false,
             "at" : "0001-01-01T00:00:00Z",
             "startedAt" : "0001-01-01T00:00:00Z",
             "clientVersion" : ""
          }
   }
}
*/

type TotalStats struct {
	At            string `json:"at"`
	InBytesTotal  int    `json:"inBytesTotal"`
	OutBytesTotal int    `json:"outBytesTotal"`
}

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

type SystemConnections struct {
	Total       TotalStats                 `json:"total"`
	Connections map[string]ConnectionStats `json:"connections"`
}

type APIConfig struct {
	APIURL string `json:"apiURL"`
	APIKey string `json:"apiKey"`
}

type Syncthing struct {
	SystemConnections *SystemConnections `json:"systemConnections"`
	SystemStatus      *SystemStatus      `json:"systemStatus"`
	Config            *SyncthingConfig   `json:"config"`
	APIConfig         *APIConfig         `json:"apiConfig"`
}
