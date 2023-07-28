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
	"github.com/spf13/viper"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/controllers/mover"
	"github.com/backube/volsync/controllers/utils"
	"github.com/backube/volsync/controllers/volumehandler"
)

const (
	// defaultRcloneContainerImage is the default container image for the rclone
	// data mover
	defaultRcloneContainerImage = "quay.io/backube/volsync:latest"
	// Command line flag will be checked first
	// If command line flag not set, the RELATED_IMAGE_ env var will be used
	rcloneContainerImageFlag   = "rclone-container-image"
	rcloneContainerImageEnvVar = "RELATED_IMAGE_RCLONE_CONTAINER"
)

type Builder struct {
	viper *viper.Viper  // For unit tests to be able to override - global viper will be used by default in Register()
	flags *flag.FlagSet // For unit tests to be able to override - global flags will be used by default in Register()
}

var _ mover.Builder = &Builder{}

func Register() error {
	// Use global viper & command line flags
	b, err := newBuilder(viper.GetViper(), flag.CommandLine)
	if err != nil {
		return err
	}

	mover.Register(b)
	return nil
}

func newBuilder(viper *viper.Viper, flags *flag.FlagSet) (*Builder, error) {
	b := &Builder{
		viper: viper,
		flags: flags,
	}

	// Set default rclone container image - will be used if both command line flag and env var are not set
	b.viper.SetDefault(rcloneContainerImageFlag, defaultRcloneContainerImage)

	// Setup command line flag for the rclone container image
	b.flags.String(rcloneContainerImageFlag, defaultRcloneContainerImage,
		"The container image for the rclone data mover")
	// Viper will check for command line flag first, then fallback to the env var
	err := b.viper.BindEnv(rcloneContainerImageFlag, rcloneContainerImageEnvVar)

	return b, err
}

func (rb *Builder) VersionInfo() string {
	return fmt.Sprintf("Rclone container: %s", rb.getRcloneContainerImage())
}

// rcloneContainerImage is the container image name of the rclone data mover
func (rb *Builder) getRcloneContainerImage() string {
	return rb.viper.GetString(rcloneContainerImageFlag)
}

// nolint: funlen
func (rb *Builder) FromSource(client client.Client, logger logr.Logger,
	eventRecorder events.EventRecorder,
	source *volsyncv1alpha1.ReplicationSource, privileged bool) (mover.Mover, error) {
	// Only build if the CR belongs to us
	if source.Spec.Rclone == nil {
		return nil, nil
	}

	if source.Status.LatestMoverStatus == nil {
		source.Status.LatestMoverStatus = &volsyncv1alpha1.MoverStatus{}
	}

	vh, err := volumehandler.NewVolumeHandler(
		volumehandler.WithClient(client),
		volumehandler.WithRecorder(eventRecorder),
		volumehandler.WithOwner(source),
		volumehandler.FromSource(&source.Spec.Rclone.ReplicationSourceVolumeOptions),
	)
	if err != nil {
		return nil, err
	}

	isSource := true

	saHandler := utils.NewSAHandler(client, source, isSource, privileged,
		source.Spec.Rclone.MoverServiceAccount)

	isImmediate := true
	if source.Spec.Immediate != nil {
		isImmediate = *source.Spec.Immediate
	}

	return &Mover{
		client:               client,
		logger:               logger.WithValues("method", "Rclone"),
		eventRecorder:        eventRecorder,
		owner:                source,
		vh:                   vh,
		saHandler:            saHandler,
		containerImage:       rb.getRcloneContainerImage(),
		rcloneConfigSection:  source.Spec.Rclone.RcloneConfigSection,
		rcloneDestPath:       source.Spec.Rclone.RcloneDestPath,
		rcloneConfig:         source.Spec.Rclone.RcloneConfig,
		isSource:             isSource,
		paused:               source.Spec.Paused,
		mainPVCName:          &source.Spec.SourcePVC,
		customCASpec:         source.Spec.Rclone.CustomCA,
		privileged:           privileged,
		moverSecurityContext: source.Spec.Rclone.MoverSecurityContext,
		latestMoverStatus:    source.Status.LatestMoverStatus,
		isImmediate:          isImmediate,
	}, nil
}

func (rb *Builder) FromDestination(client client.Client, logger logr.Logger,
	eventRecorder events.EventRecorder,
	destination *volsyncv1alpha1.ReplicationDestination, privileged bool) (mover.Mover, error) {
	// Only build if the CR belongs to us
	if destination.Spec.Rclone == nil {
		return nil, nil
	}

	if destination.Status.LatestMoverStatus == nil {
		destination.Status.LatestMoverStatus = &volsyncv1alpha1.MoverStatus{}
	}

	vh, err := volumehandler.NewVolumeHandler(
		volumehandler.WithClient(client),
		volumehandler.WithRecorder(eventRecorder),
		volumehandler.WithOwner(destination),
		volumehandler.FromDestination(&destination.Spec.Rclone.ReplicationDestinationVolumeOptions),
	)
	if err != nil {
		return nil, err
	}

	isSource := false

	saHandler := utils.NewSAHandler(client, destination, isSource, privileged,
		destination.Spec.Rclone.MoverServiceAccount)

	return &Mover{
		client:               client,
		logger:               logger.WithValues("method", "Rclone"),
		eventRecorder:        eventRecorder,
		owner:                destination,
		vh:                   vh,
		saHandler:            saHandler,
		containerImage:       rb.getRcloneContainerImage(),
		rcloneConfigSection:  destination.Spec.Rclone.RcloneConfigSection,
		rcloneDestPath:       destination.Spec.Rclone.RcloneDestPath,
		rcloneConfig:         destination.Spec.Rclone.RcloneConfig,
		isSource:             isSource,
		paused:               destination.Spec.Paused,
		mainPVCName:          destination.Spec.Rclone.DestinationPVC,
		customCASpec:         destination.Spec.Rclone.CustomCA,
		privileged:           privileged,
		moverSecurityContext: destination.Spec.Rclone.MoverSecurityContext,
		latestMoverStatus:    destination.Status.LatestMoverStatus,
	}, nil
}
