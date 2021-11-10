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

package rclone

import (
	"flag"
	"fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/controllers/mover"
	"github.com/backube/volsync/controllers/volumehandler"
)

// defaultRcloneContainerImage is the default container image for the rclone
// data mover
const defaultRcloneContainerImage = "quay.io/backube/volsync-mover-rclone:latest"

// rcloneContainerImage is the container image name of the rclone data mover
var rcloneContainerImage string

type Builder struct{}

var _ mover.Builder = &Builder{}

func Register() {
	flag.StringVar(&rcloneContainerImage, "rclone-container-image",
		defaultRcloneContainerImage, "The container image for the rclone data mover")
	mover.Register(&Builder{})
}

func (rb *Builder) VersionInfo() string {
	return fmt.Sprintf("Rclone container: %s", rcloneContainerImage)
}

func (rb *Builder) FromSource(client client.Client, logger logr.Logger,
	source *volsyncv1alpha1.ReplicationSource) (mover.Mover, error) {
	// Only build if the CR belongs to us
	if source.Spec.Rclone == nil {
		return nil, nil
	}

	vh, err := volumehandler.NewVolumeHandler(
		volumehandler.WithClient(client),
		volumehandler.WithOwner(source),
		volumehandler.FromSource(&source.Spec.Rclone.ReplicationSourceVolumeOptions),
	)
	if err != nil {
		return nil, err
	}

	return &Mover{
		client:              client,
		logger:              logger.WithValues("method", "Rclone"),
		owner:               source,
		vh:                  vh,
		rcloneConfigSection: source.Spec.Rclone.RcloneConfigSection,
		rcloneDestPath:      source.Spec.Rclone.RcloneDestPath,
		rcloneConfig:        source.Spec.Rclone.RcloneConfig,
		isSource:            true,
		paused:              source.Spec.Paused,
		mainPVCName:         &source.Spec.SourcePVC,
	}, nil
}

func (rb *Builder) FromDestination(client client.Client, logger logr.Logger,
	destination *volsyncv1alpha1.ReplicationDestination) (mover.Mover, error) {
	// Only build if the CR belongs to us
	if destination.Spec.Rclone == nil {
		return nil, nil
	}

	vh, err := volumehandler.NewVolumeHandler(
		volumehandler.WithClient(client),
		volumehandler.WithOwner(destination),
		volumehandler.FromDestination(&destination.Spec.Rclone.ReplicationDestinationVolumeOptions),
	)
	if err != nil {
		return nil, err
	}

	return &Mover{
		client:              client,
		logger:              logger.WithValues("method", "Rclone"),
		owner:               destination,
		vh:                  vh,
		rcloneConfigSection: destination.Spec.Rclone.RcloneConfigSection,
		rcloneDestPath:      destination.Spec.Rclone.RcloneDestPath,
		rcloneConfig:        destination.Spec.Rclone.RcloneConfig,
		isSource:            false,
		paused:              destination.Spec.Paused,
		mainPVCName:         destination.Spec.Rclone.DestinationPVC,
		destStatus:          destination.Status,
	}, nil
}
