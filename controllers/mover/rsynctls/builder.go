//go:build !disable_rsynctls

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

package rsynctls

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
	rsyncTLSMoverName = "rsync-tls"
	// defaultRsyncContainerImage is the default container image for the
	// rsync-tls data mover
	defaultRsyncTLSContainerImage = "quay.io/backube/volsync:latest"
	// Command line flag will be checked first
	// If command line flag not set, the RELATED_IMAGE_ env var will be used
	rsyncTLSContainerImageFlag   = "rsync-tls-container-image"
	rsyncTLSContainerImageEnvVar = "RELATED_IMAGE_RSYNC_TLS_CONTAINER"
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

	// Set default rsync-tls container image - will be used if both command line flag and env var are not set
	b.viper.SetDefault(rsyncTLSContainerImageFlag, defaultRsyncTLSContainerImage)

	// Setup command line flag for the rsync-tls container image
	b.flags.String(rsyncTLSContainerImageFlag, defaultRsyncTLSContainerImage,
		"The container image for the rsync-tls data mover")
	// Viper will check for command line flag first, then fallback to the env var
	err := b.viper.BindEnv(rsyncTLSContainerImageFlag, rsyncTLSContainerImageEnvVar)

	return b, err
}

func (rb *Builder) Name() string { return rsyncTLSMoverName }

func (rb *Builder) VersionInfo() string {
	return fmt.Sprintf("Rsync TLS container: %s", rb.getRsyncTLSContainerImage())
}

// rsyncTLSContainerImage is the container image name of the rsync-tls data mover
func (rb *Builder) getRsyncTLSContainerImage() string {
	return rb.viper.GetString(rsyncTLSContainerImageFlag)
}

func (rb *Builder) FromSource(client client.Client, logger logr.Logger,
	eventRecorder events.EventRecorder,
	source *volsyncv1alpha1.ReplicationSource, privileged bool) (mover.Mover, error) {
	// Only build if the CR belongs to us
	if source.Spec.RsyncTLS == nil {
		return nil, nil
	}

	// Make sure there's a place to write status info
	if source.Status.RsyncTLS == nil {
		source.Status.RsyncTLS = &volsyncv1alpha1.ReplicationSourceRsyncTLSStatus{}
	}

	if source.Status.LatestMoverStatus == nil {
		source.Status.LatestMoverStatus = &volsyncv1alpha1.MoverStatus{}
	}

	vh, err := volumehandler.NewVolumeHandler(
		volumehandler.WithClient(client),
		volumehandler.WithRecorder(eventRecorder),
		volumehandler.WithOwner(source),
		volumehandler.FromSource(&source.Spec.RsyncTLS.ReplicationSourceVolumeOptions),
	)
	if err != nil {
		return nil, err
	}

	isSource := true

	saHandler := utils.NewSAHandler(client, source, isSource, privileged,
		source.Spec.RsyncTLS.MoverServiceAccount)

	return &Mover{
		client:             client,
		logger:             logger.WithValues("method", "RsyncTLS"),
		eventRecorder:      eventRecorder,
		owner:              source,
		vh:                 vh,
		saHandler:          saHandler,
		containerImage:     rb.getRsyncTLSContainerImage(),
		key:                source.Spec.RsyncTLS.KeySecret,
		serviceType:        nil,
		serviceAnnotations: nil,
		address:            source.Spec.RsyncTLS.Address,
		port:               source.Spec.RsyncTLS.Port,
		isSource:           isSource,
		paused:             source.Spec.Paused,
		mainPVCName:        &source.Spec.SourcePVC,
		privileged:         privileged,
		sourceStatus:       source.Status.RsyncTLS,
		latestMoverStatus:  source.Status.LatestMoverStatus,
		moverConfig:        source.Spec.RsyncTLS.MoverConfig,
	}, nil
}

//nolint:funlen
func (rb *Builder) FromDestination(client client.Client, logger logr.Logger,
	eventRecorder events.EventRecorder,
	destination *volsyncv1alpha1.ReplicationDestination, privileged bool) (mover.Mover, error) {
	// Only build if the CR belongs to us
	if destination.Spec.RsyncTLS == nil {
		return nil, nil
	}

	// Make sure there's a place to write status info
	if destination.Status.RsyncTLS == nil {
		destination.Status.RsyncTLS = &volsyncv1alpha1.ReplicationDestinationRsyncTLSStatus{}
	}

	if destination.Status.LatestMoverStatus == nil {
		destination.Status.LatestMoverStatus = &volsyncv1alpha1.MoverStatus{}
	}

	vh, err := volumehandler.NewVolumeHandler(
		volumehandler.WithClient(client),
		volumehandler.WithRecorder(eventRecorder),
		volumehandler.WithOwner(destination),
		volumehandler.FromDestination(&destination.Spec.RsyncTLS.ReplicationDestinationVolumeOptions),
	)
	if err != nil {
		return nil, err
	}

	isSource := false

	saHandler := utils.NewSAHandler(client, destination, isSource, privileged,
		destination.Spec.RsyncTLS.MoverServiceAccount)

	var svcAnnotations map[string]string
	if destination.Spec.RsyncTLS.ServiceAnnotations != nil {
		// If nil we will assume VolSync can set defaults
		// if not nil, we will assume we will use the users settings (and empty map will mean
		// we do not set any annotations at all on the service)
		svcAnnotations = *destination.Spec.RsyncTLS.ServiceAnnotations
	}

	return &Mover{
		client:                    client,
		logger:                    logger.WithValues("method", "RsyncTLS"),
		eventRecorder:             eventRecorder,
		owner:                     destination,
		vh:                        vh,
		saHandler:                 saHandler,
		containerImage:            rb.getRsyncTLSContainerImage(),
		key:                       destination.Spec.RsyncTLS.KeySecret,
		serviceType:               destination.Spec.RsyncTLS.ServiceType,
		serviceAnnotations:        svcAnnotations,
		address:                   nil,
		port:                      nil,
		isSource:                  isSource,
		paused:                    destination.Spec.Paused,
		mainPVCName:               destination.Spec.RsyncTLS.DestinationPVC,
		cleanupTempDestinationPVC: destination.Spec.RsyncTLS.CleanupTempDestinationPVC,
		privileged:                privileged,
		destStatus:                destination.Status.RsyncTLS,
		latestMoverStatus:         destination.Status.LatestMoverStatus,
		moverConfig:               destination.Spec.RsyncTLS.MoverConfig,
	}, nil
}
