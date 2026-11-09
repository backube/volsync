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
package syncthing

import (
	"crypto/rand"
	"regexp"

	"github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/internal/controller/mover/syncthing/api"
	"github.com/backube/volsync/internal/controller/mover/syncthing/lib/config"
)

// updateSyncthingDevices reconciles the daemon's device list and folder sharing
// with the given peerList using granular REST calls. It removes stale devices
// (skipping self and introduced ones), upserts the desired peers, and re-shares
// folders with self, introduced devices, and the peerList.
func updateSyncthingDevices(
	connection api.SyncthingConnection,
	peerList []v1alpha1.SyncthingPeer,
	syncthing *api.Syncthing,
) error {
	// remove stale managed devices that are not in the peerList
	for _, device := range managedDevices(syncthing) {
		if _, wanted := desiredPeerIDs(peerList)[device.DeviceID]; !wanted {
			if err := connection.RemoveDevice(device.DeviceID); err != nil {
				return err
			}
		}
	}

	// add/update devices from peerList
	for _, peer := range peerList {
		if err := connection.AddOrUpdateDevice(config.DeviceConfiguration{
			DeviceID:   peer.ID,
			Addresses:  []string{peer.Address},
			Introducer: peer.Introducer,
		}); err != nil {
			return err
		}
	}

	folderDevices := expectedFolderDevices(peerList, syncthing)

	// update folder sharing
	for _, folder := range syncthing.Configuration.Folders {
		if err := connection.PatchFolderDevices(folder.ID, folderDevices); err != nil {
			return err
		}
	}
	return nil
}

// syncthingNeedsReconfigure Determines whether the given nodeList or folder sharing
// diverges from Syncthing's current configuration, and returns 'true' if the
// Syncthing API must be reconfigured, 'false' otherwise.
func syncthingNeedsReconfigure(
	nodeList []v1alpha1.SyncthingPeer,
	syncthing *api.Syncthing,
) bool {
	// check if the syncthing nodelist diverges from the current syncthing devices
	var newDevices = map[string]v1alpha1.SyncthingPeer{
		// initialize the map with the self node
		syncthing.MyID(): {
			ID:      syncthing.MyID(),
			Address: "",
		},
	}

	// add all of the other devices in the provided nodeList
	for _, device := range nodeList {
		// avoid self
		if device.ID == syncthing.MyID() {
			continue
		}
		newDevices[device.ID] = device
	}

	// create a map for current devices
	var currentDevs = map[string]v1alpha1.SyncthingPeer{
		// initialize the map with the self node
		syncthing.MyID(): {
			ID:      syncthing.MyID(),
			Address: "",
		},
	}
	// add the managed devices to the map
	for _, device := range managedDevices(syncthing) {
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

	// check if folder sharing diverges from the expected device lists, so that
	// a previously failed folder update is retried on a later reconcile
	expected := expectedFolderDevices(nodeList, syncthing)
	for _, folder := range syncthing.Configuration.Folders {
		if !folderDevicesEqual(folder.Devices, expected) {
			return true
		}
	}
	return false
}

// managedDevices returns the daemon's current devices excluding self and
// devices introduced by an introducer. These are the devices whose membership
// this mover is responsible for keeping in sync with the peerList.
func managedDevices(syncthing *api.Syncthing) []config.DeviceConfiguration {
	managed := make([]config.DeviceConfiguration, 0, len(syncthing.Configuration.Devices))
	for _, device := range syncthing.Configuration.Devices {
		if device.DeviceID == syncthing.MyID() || device.IntroducedBy != "" {
			continue
		}
		managed = append(managed, device)
	}
	return managed
}

// desiredPeerIDs returns the set of device IDs that the peerList says should
// exist in the daemon's configuration.
func desiredPeerIDs(peerList []v1alpha1.SyncthingPeer) map[string]struct{} {
	desired := make(map[string]struct{}, len(peerList))
	for _, peer := range peerList {
		desired[peer.ID] = struct{}{}
	}
	return desired
}

// expectedFolderDevices returns the device list every folder should be shared
func expectedFolderDevices(
	peerList []v1alpha1.SyncthingPeer,
	syncthing *api.Syncthing,
) []config.FolderDeviceConfiguration {
	expected := []config.FolderDeviceConfiguration{
		{DeviceID: syncthing.MyID()},
	}
	for _, device := range syncthing.Configuration.Devices {
		if device.IntroducedBy != "" {
			expected = append(expected, config.FolderDeviceConfiguration{
				DeviceID:     device.DeviceID,
				IntroducedBy: device.IntroducedBy,
			})
		}
	}
	for _, peer := range peerList {
		expected = append(expected, config.FolderDeviceConfiguration{DeviceID: peer.ID})
	}
	return expected
}

// folderDevicesEqual reports whether two folder device lists contain the same
// device IDs, regardless of order.
func folderDevicesEqual(a, b []config.FolderDeviceConfiguration) bool {
	if len(a) != len(b) {
		return false
	}
	ids := make(map[string]struct{}, len(a))
	for _, device := range a {
		ids[device.DeviceID] = struct{}{}
	}
	for _, device := range b {
		if _, ok := ids[device.DeviceID]; !ok {
			return false
		}
	}
	return true
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
	var acceptableRange = upperBound - lowerBound + 1

	// generate the string by mapping [0, 255] -> [33, 126]
	var acceptableBytes = []byte{}
	for i := 0; i < len(b); i++ {
		// normalize number to be in the range [33, 126] inclusive
		acceptableByte := (b[i] % acceptableRange) + lowerBound
		acceptableBytes = append(acceptableBytes, acceptableByte)
	}
	return string(acceptableBytes), nil
}

// asTCPAddress Accepts an address of some form and returns it with a TCP prefix if none exist yet.//
// If the address already contains a prefix, then it is simply returned.
//
// See: https://forum.syncthing.net/t/specifying-protocols-without-global-announce-or-relay/18565
func asTCPAddress(address string) string {
	// ignore if a prefix already exists
	uriPattern := regexp.MustCompile(`^(\w+:\/\/)[^\s]+$`)
	if uriPattern.MatchString(address) {
		return address
	}

	return "tcp://" + address //nolint:goconst // goconst thinks tcp:// is used 4x
}
