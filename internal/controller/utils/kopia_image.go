/*
Copyright 2025 The VolSync authors.

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

package utils

import "os"

const (
	// defaultKopiaContainerImage is the default container image for the kopia data mover
	defaultKopiaContainerImage = "quay.io/backube/volsync:latest"
	// kopiaContainerImageEnvVar is the environment variable for the kopia container image
	kopiaContainerImageEnvVar = "RELATED_IMAGE_KOPIA_CONTAINER"
)

// GetDefaultKopiaImage returns the default Kopia container image
// It checks the environment variable first, then falls back to the default
func GetDefaultKopiaImage() string {
	if image := os.Getenv(kopiaContainerImageEnvVar); image != "" {
		return image
	}
	return defaultKopiaContainerImage
}

// GetKopiaImageEnvVar returns the environment variable name for the Kopia container image
func GetKopiaImageEnvVar() string {
	return kopiaContainerImageEnvVar
}