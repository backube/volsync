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

import "github.com/syncthing/syncthing/lib/config"

// GetDeviceFromID Returns a pointer to the device with the given ID,
// along with a boolean indicating whether the device was found.
func (s *Syncthing) GetDeviceFromID(id string) (*config.DeviceConfiguration, bool) {
	for _, device := range s.Configuration.Devices {
		if device.DeviceID.GoString() == id {
			return &device, true
		}
	}
	return nil, false
}

// MyID Is a convenience method which returns the current Syncthing device's ID.
func (s *Syncthing) MyID() string { return s.SystemStatus.MyID }

// ShareFoldersWithDevices Will set all of the given devices to be shared with the
// currently tracked folders.
//
// This method does not currently take into account any encryption password set
// on the folder by the device.
func (s *Syncthing) ShareFoldersWithDevices(devices []config.DeviceConfiguration) {
	// share the current folder(s) with the new devices
	var newFolders = []config.FolderConfiguration{}
	for _, folder := range s.Configuration.Folders {
		// copy folder & reset
		newFolder := folder
		newFolder.Devices = []config.FolderDeviceConfiguration{}

		for _, device := range s.Configuration.Devices {
			newFolder.Devices = append(newFolder.Devices, config.FolderDeviceConfiguration{
				DeviceID:     device.DeviceID,
				IntroducedBy: device.IntroducedBy,
			})
		}
		newFolders = append(newFolders, newFolder)
	}
	s.Configuration.Folders = newFolders
}
