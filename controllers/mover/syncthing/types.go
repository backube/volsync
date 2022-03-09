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

import "github.com/go-logr/logr"

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
	ListenAddresses                     []string      `json:"listenAddresses"`
	GlobalAnnounceServers               []string      `json:"globalAnnounceServers"`
	GlobalAnnounceEnabled               bool          `json:"globalAnnounceEnabled"`
	LocalAnnounceEnabled                bool          `json:"localAnnounceEnabled"`
	LocalAnnouncePort                   int           `json:"localAnnouncePort"`
	LocalAnnounceMCAddr                 string        `json:"localAnnounceMCAddr"`
	MaxSendKbps                         int           `json:"maxSendKbps"`
	MaxRecvKbps                         int           `json:"maxRecvKbps"`
	ReconnectionIntervalS               int           `json:"reconnectionIntervalS"`
	RelaysEnabled                       bool          `json:"relaysEnabled"`
	RelayReconnectIntervalM             int           `json:"relayReconnectIntervalM"`
	StartBrowser                        bool          `json:"startBrowser"`
	NATEnabled                          bool          `json:"natEnabled"`
	NATLeaseMinutes                     int           `json:"natLeaseMinutes"`
	NATRenewalMinutes                   int           `json:"natRenewalMinutes"`
	NATTimeoutSeconds                   int           `json:"natTimeoutSeconds"`
	URAccepted                          int           `json:"urAccepted"`
	URSeen                              int           `json:"urSeen"`
	URUniqueId                          string        `json:"urUniqueId"`
	URURL                               string        `json:"urURL"`
	URPostInsecurely                    bool          `json:"urPostInsecurely"`
	URInitialDelayS                     int           `json:"urInitialDelayS"`
	RestartOnWakeup                     bool          `json:"restartOnWakeup"`
	AutoUpgradeIntervalH                int           `json:"autoUpgradeIntervalH"`
	UpgradeToPreReleases                bool          `json:"upgradeToPreReleases"`
	KeepTemporariesH                    int           `json:"keepTemporariesH"`
	CacheIgnoredFiles                   bool          `json:"cacheIgnoredFiles"`
	ProgressUpdateIntervalS             int           `json:"progressUpdateIntervalS"`
	LimitBandwidthLan                   bool          `json:"limitBandwidthLan"`
	MinHomeDiskFree                     SyncthingSize `json:"minHomeDiskFree"`
	ReleasesURL                         string        `json:"releasesURL"`
	AlwaysLocalNets                     []string      `json:"alwaysLocalNets"`
	OverwriteRemoteDeviceNamesOnConnect bool          `json:"overwriteRemoteDeviceNamesOnConnect"`
	TempIndexMinBlocks                  int           `json:"tempIndexMinBlocks"`
	UnackedNotificationIDs              []string      `json:"unackedNotificationIDs"`
	TrafficClass                        int           `json:"trafficClass"`
	SetLowPriority                      bool          `json:"setLowPriority"`
	MaxFolderConcurrency                int           `json:"maxFolderConcurrency"`
	CRURL                               string        `json:"crURL"`
	CrashReportingEnabled               bool          `json:"crashReportingEnabled"`
	StunKeepaliveStartS                 int           `json:"stunKeepaliveStartS"`
	StunKeepaliveMinS                   int           `json:"stunKeepaliveMinS"`
	StunServers                         []string      `json:"stunServers"`
	DatabaseTuning                      string        `json:"databaseTuning"`
	MaxConcurrentIncomingRequestKiB     int           `json:"maxConcurrentIncomingRequestKiB"`
	AnnounceLANAddresses                bool          `json:"announceLANAddresses"`
	SendFullIndexOnUpgrade              bool          `json:"sendFullIndexOnUpgrade"`
	FeatureFlags                        []string      `json:"featureFlags"`
	ConnectionLimitEnough               int           `json:"connectionLimitEnough"`
	ConnectionLimitMax                  int           `json:"connectionLimitMax"`
	InsecureAllowOldTLSVersions         bool          `json:"insecureAllowOldTLSVersions"`
}

type FolderDeviceConfiguration struct {
	DeviceID           string `json:"deviceID"`
	IntroducedBy       string `json:"introducedBy"`
	EncryptionPassword string `json:"encryptionPassword"`
}

type Defaults struct {
	Folder SyncthingFolder `json:"folder"`
	Device SyncthingDevice `json:"device"`
}

type Ldap struct {
	Address            string `json:"address"`
	BindDN             string `json:"bindDN"`
	Transport          string `json:"transport"`
	InsecureSkipVerify bool   `json:"insecureSkipVerify"`
	SearchBaseDN       string `json:"searchBaseDN"`
	SearchFilter       string `json:"searchFilter"`
}

type Gui struct {
	Enabled                   bool   `json:"enabled"`
	Address                   string `json:"address"`
	UnixSocketPermissions     string `json:"unixSocketPermissions"`
	User                      string `json:"user"`
	Password                  string `json:"password"`
	AuthMode                  string `json:"authMode"`
	UseTLS                    bool   `json:"useTLS"`
	APIKey                    string `json:"apiKey"`
	InsecureAdminAccess       bool   `json:"insecureAdminAccess"`
	Theme                     string `json:"theme"`
	Debugging                 bool   `json:"debugging"`
	InsecureSkipHostcheck     bool   `json:"insecureSkipHostcheck"`
	InsecureAllowFrameLoading bool   `json:"insecureAllowFrameLoading"`
}

//nolint:revive
type SyncthingConfig struct {
	Version              int                      `json:"version"`
	Folders              []SyncthingFolder        `json:"folders"`
	Devices              []SyncthingDevice        `json:"devices"`
	Options              SyncthingOptions         `json:"options"`
	Device               SyncthingDevice          `json:"device"`
	RemoteIgnoredDevices []GenericSyncthingDevice `json:"remoteIgnoredDevices"`
	LDAP                 Ldap                     `json:"ldap"`
	GUI                  Gui                      `json:"gui"`
	Defaults             Defaults                 `json:"defaults"`
}

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

type GenericSyncthingDevice struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Address string `json:"address"`
	Time    string `json:"time"`
}

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
	APIURL      string `json:"apiURL"`
	APIKey      string `json:"apiKey"`
	GUIUser     string `json:"user"`
	GUIPassword string `json:"password"`
}

type Syncthing struct {
	SystemConnections *SystemConnections `json:"systemConnections"`
	SystemStatus      *SystemStatus      `json:"systemStatus"`
	Config            *SyncthingConfig   `json:"config"`
	APIConfig         *APIConfig         `json:"apiConfig"`
	logger            logr.Logger
}
