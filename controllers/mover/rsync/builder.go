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

package rsync

import (
	"flag"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/spf13/viper"
	"sigs.k8s.io/controller-runtime/pkg/client"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/controllers/mover"
	"github.com/backube/volsync/controllers/volumehandler"
)

const (
	// defaultRsyncContainerImage is the default container image for the rsync
	// data mover
	defaultRsyncContainerImage = "quay.io/backube/volsync-mover-rsync:latest"
	// Command line flag will be checked first
	// If command line flag not set, the RELATED_IMAGE_ env var will be used
	rsyncContainerImageFlag   = "rsync-container-image"
	rsyncContainerImageEnvVar = "RELATED_IMAGE_RSYNC_CONTAINER"
)

type Builder struct {
	viper *viper.Viper  // For unit tests to be able to override (global viper will be used if this is nil)
	flags *flag.FlagSet // For unit tests to be able to override (global flags will be used if this is nil)
}

var _ mover.Builder = &Builder{}

func Register() error {
	b := &Builder{}
	if err := b.initFlags(); err != nil {
		return err
	}

	mover.Register(b)
	return nil
}

func (rb *Builder) initFlags() error {
	// Set default rsync container image - will be used if both command line flag and env var are not set
	rb.getViper().SetDefault(rsyncContainerImageFlag, defaultRsyncContainerImage)

	// Viper will check for command line flag first, then fallback to the env var
	rb.getFlagSet().String(rsyncContainerImageFlag, defaultRsyncContainerImage,
		"The container image for the rsync data mover")
	return rb.getViper().BindEnv(rsyncContainerImageFlag, rsyncContainerImageEnvVar)
}

func (rb *Builder) VersionInfo() string {
	return fmt.Sprintf("Rsync container: %s", rb.getRsyncContainerImage())
}

func (rb *Builder) getViper() *viper.Viper {
	if rb.viper == nil {
		rb.viper = viper.GetViper() // Use Global viper
	}
	return rb.viper
}

func (rb *Builder) getFlagSet() *flag.FlagSet {
	if rb.flags == nil {
		rb.flags = flag.CommandLine // Use global command line flags
	}
	return rb.flags
}

// rsyncContainerImage is the container image name of the rsync data mover
func (rb *Builder) getRsyncContainerImage() string {
	return rb.getViper().GetString(rsyncContainerImageFlag)
}

func (rb *Builder) FromSource(client client.Client, logger logr.Logger,
	source *volsyncv1alpha1.ReplicationSource) (mover.Mover, error) {
	// Only build if the CR belongs to us
	if source.Spec.Rsync == nil {
		return nil, nil
	}

	// Make sure there's a place to write status info
	if source.Status.Rsync == nil {
		source.Status.Rsync = &volsyncv1alpha1.ReplicationSourceRsyncStatus{}
	}

	vh, err := volumehandler.NewVolumeHandler(
		volumehandler.WithClient(client),
		volumehandler.WithOwner(source),
		volumehandler.FromSource(&source.Spec.Rsync.ReplicationSourceVolumeOptions),
	)
	if err != nil {
		return nil, err
	}

	return &Mover{
		client:         client,
		logger:         logger.WithValues("method", "Rsync"),
		owner:          source,
		vh:             vh,
		containerImage: rb.getRsyncContainerImage(),
		sshKeys:        source.Spec.Rsync.SSHKeys,
		serviceType:    source.Spec.Rsync.ServiceType,
		address:        source.Spec.Rsync.Address,
		port:           source.Spec.Rsync.Port,
		isSource:       true,
		paused:         source.Spec.Paused,
		mainPVCName:    &source.Spec.SourcePVC,
		sourceStatus:   source.Status.Rsync,
	}, nil
}

func (rb *Builder) FromDestination(client client.Client, logger logr.Logger,
	destination *volsyncv1alpha1.ReplicationDestination) (mover.Mover, error) {
	// Only build if the CR belongs to us
	if destination.Spec.Rsync == nil {
		return nil, nil
	}

	// Make sure there's a place to write status info
	if destination.Status.Rsync == nil {
		destination.Status.Rsync = &volsyncv1alpha1.ReplicationDestinationRsyncStatus{}
	}

	vh, err := volumehandler.NewVolumeHandler(
		volumehandler.WithClient(client),
		volumehandler.WithOwner(destination),
		volumehandler.FromDestination(&destination.Spec.Rsync.ReplicationDestinationVolumeOptions),
	)
	if err != nil {
		return nil, err
	}

	return &Mover{
		client:         client,
		logger:         logger.WithValues("method", "Rsync"),
		owner:          destination,
		vh:             vh,
		containerImage: rb.getRsyncContainerImage(),
		sshKeys:        destination.Spec.Rsync.SSHKeys,
		serviceType:    destination.Spec.Rsync.ServiceType,
		address:        destination.Spec.Rsync.Address,
		port:           destination.Spec.Rsync.Port,
		isSource:       false,
		paused:         destination.Spec.Paused,
		mainPVCName:    destination.Spec.Rsync.DestinationPVC,
		destStatus:     destination.Status.Rsync,
	}, nil
}
