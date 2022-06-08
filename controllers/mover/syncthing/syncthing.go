package syncthing

import (
	"crypto/rand"
	"strings"

	"github.com/backube/volsync/api/v1alpha1"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/protocol"
)

// updateSyncthingDevices Updates the Syncthing's connected devices with the provided peerList.
// An error may be encountered when reading the DeviceID from a string.
func (m *Mover) updateSyncthingDevices(peerList []v1alpha1.SyncthingPeer) error {
	stConfig := m.syncthing.Config()
	newDevices := []config.DeviceConfiguration{}
	// add myself and introduced devices to the device list
	for _, device := range stConfig.Devices {
		if device.DeviceID.GoString() == m.syncthing.MyID() || device.IntroducedBy.GoString() != "" {
			newDevices = append(newDevices, device)
		}
	}
	// Add the devices from the peerList to the device list
	for _, device := range peerList {
		deviceID, err := protocol.DeviceIDFromString(device.ID)
		if err != nil {
			return err
		}
		stDeviceToAdd := config.DeviceConfiguration{
			DeviceID:   deviceID,
			Addresses:  []string{device.Address},
			Introducer: device.Introducer,
		}
		newDevices = append(newDevices, stDeviceToAdd)
	}
	// update the config w/ the new devices
	stConfig.Devices = newDevices
	m.syncthing.SetConfig(stConfig)
	// update folders with the new devices
	m.updateFolders()
	return nil
}

// updateFolders Updates all of Syncthing's folders to be shared with all configured devices.
func (m *Mover) updateFolders() {
	// share the current folder(s) with the new devices
	var newFolders = []config.FolderConfiguration{}
	var stConfig = m.syncthing.Config()
	for _, folder := range stConfig.Folders {
		// copy folder & reset
		newFolder := folder
		newFolder.Devices = []config.FolderDeviceConfiguration{}

		for _, device := range stConfig.Devices {
			newFolder.Devices = append(newFolder.Devices, config.FolderDeviceConfiguration{
				DeviceID:     device.DeviceID,
				IntroducedBy: device.IntroducedBy,
			})
		}
		newFolders = append(newFolders, newFolder)
	}
	stConfig.Folders = newFolders
	m.syncthing.SetConfig(stConfig)
}

// syncthingNeedsReconfigure Determines whether the given nodeList differs from Syncthing's internal devices,
// and returns 'true' if the Syncthing API must be reconfigured, 'false' otherwise.
func (m *Mover) syncthingNeedsReconfigure(nodeList []v1alpha1.SyncthingPeer) bool {
	var stConfig = m.syncthing.Config()
	// check if the syncthing nodelist diverges from the current syncthing devices
	var newDevices map[string]v1alpha1.SyncthingPeer = map[string]v1alpha1.SyncthingPeer{
		// initialize the map with the self node
		m.syncthing.MyID(): {
			ID:      m.syncthing.MyID(),
			Address: "",
		},
	}

	// add all of the other devices in the provided nodeList
	for _, device := range nodeList {
		// avoid self
		if device.ID == m.syncthing.MyID() {
			continue
		}
		newDevices[device.ID] = device
	}

	// create a map for current devices
	var currentDevs map[string]v1alpha1.SyncthingPeer = map[string]v1alpha1.SyncthingPeer{
		// initialize the map with the self node
		m.syncthing.MyID(): {
			ID:      m.syncthing.MyID(),
			Address: "",
		},
	}
	// add the rest of devices to the map
	for _, device := range stConfig.Devices {
		// ignore self and introduced devices
		if device.DeviceID.GoString() == m.syncthing.MyID() || device.IntroducedBy.GoString() != "" {
			continue
		}

		currentDevs[device.DeviceID.GoString()] = v1alpha1.SyncthingPeer{
			ID:      device.DeviceID.GoString(),
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

// peerListContainsIntroduced Returns 'true' if the given peerList contains a node
// which has been introduced to us by another Syncthing instance, 'false' otherwise.
func (m *Mover) peerListContainsIntroduced(peerList []v1alpha1.SyncthingPeer) bool {
	introducedSet := m.syncthing.IntroducedDevices()

	// check if the peerList contains an introduced node
	for _, peer := range peerList {
		if _, ok := introducedSet[peer.ID]; ok {
			return true
		}
	}
	return false
}

// peerListContainsSelf Returns 'true' if the given peerList contains the self node, 'false' otherwise.
func (m *Mover) peerListContainsSelf(peerList []v1alpha1.SyncthingPeer) bool {
	for _, peer := range peerList {
		if peer.ID == m.syncthing.MyID() {
			return true
		}
	}
	return false
}

// getDeviceFromID Returns the device with the given ID,
// along with a boolean indicating whether the device was found.
func (m *Mover) getDeviceFromID(deviceID string) (config.DeviceConfiguration, bool) {
	for _, device := range m.syncthing.Config().Devices {
		if device.DeviceID.GoString() == deviceID {
			return device, true
		}
	}
	return config.DeviceConfiguration{}, false
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

// asTCPAddress Accepts a partial URL which may be a hostname or a hostname:port, and returns a TCP address.
// If the provided address already contains a prefix, then it is
func asTCPAddress(addr string) string {
	// check if TCP is already prefixed
	if strings.HasPrefix(addr, "tcp://") {
		return addr
	}
	return "tcp://" + addr
}
