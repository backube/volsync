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

package restic

import (
	"flag"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/spf13/viper"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/controllers/mover"
	"github.com/backube/volsync/controllers/volumehandler"
)

const (
	// defaultResticContainerImage is the default container image for the restic
	// data mover
	defaultResticContainerImage = "quay.io/backube/volsync:latest"
	// Command line flag will be checked first
	// If command line flag not set, the RELATED_IMAGE_ env var will be used
	resticContainerImageFlag   = "restic-container-image"
	resticContainerImageEnvVar = "RELATED_IMAGE_RESTIC_CONTAINER"
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

	// Set default restic container image - will be used if both command line flag and env var are not set
	b.viper.SetDefault(resticContainerImageFlag, defaultResticContainerImage)

	// Setup command line flag for the restic container image
	b.flags.String(resticContainerImageFlag, defaultResticContainerImage,
		"The container image for the restic data mover")
	// Viper will check for command line flag first, then fallback to the env var
	err := b.viper.BindEnv(resticContainerImageFlag, resticContainerImageEnvVar)

	return b, err
}

func (rb *Builder) VersionInfo() string {
	return fmt.Sprintf("Restic container: %s", rb.getResticContainerImage())
}

// resticContainerImage is the container image name of the restic data mover
func (rb *Builder) getResticContainerImage() string {
	return rb.viper.GetString(resticContainerImageFlag)
}

func (rb *Builder) FromSource(client client.Client, logger logr.Logger,
	eventRecorder events.EventRecorder,
	source *volsyncv1alpha1.ReplicationSource, privileged bool) (mover.Mover, error) {
	// Only build if the CR belongs to us
	if source.Spec.Restic == nil {
		return nil, nil
	}

	// Create ReplicationSourceResticStatus to write restic status
	if source.Status.Restic == nil {
		source.Status.Restic = &volsyncv1alpha1.ReplicationSourceResticStatus{}
	}

	if source.Status.LatestMoverStatus == nil {
		source.Status.LatestMoverStatus = &volsyncv1alpha1.MoverStatus{}
	}

	vh, err := volumehandler.NewVolumeHandler(
		volumehandler.WithClient(client),
		volumehandler.WithRecorder(eventRecorder),
		volumehandler.WithOwner(source),
		volumehandler.FromSource(&source.Spec.Restic.ReplicationSourceVolumeOptions),
	)
	if err != nil {
		return nil, err
	}

	return &Mover{
		client:                client,
		logger:                logger.WithValues("method", "Restic"),
		eventRecorder:         eventRecorder,
		owner:                 source,
		vh:                    vh,
		containerImage:        rb.getResticContainerImage(),
		cacheAccessModes:      source.Spec.Restic.CacheAccessModes,
		cacheCapacity:         source.Spec.Restic.CacheCapacity,
		cacheStorageClassName: source.Spec.Restic.CacheStorageClassName,
		repositoryName:        source.Spec.Restic.Repository,
		isSource:              true,
		paused:                source.Spec.Paused,
		mainPVCName:           &source.Spec.SourcePVC,
		caSecretName:          source.Spec.Restic.CustomCA.SecretName,
		caSecretKey:           source.Spec.Restic.CustomCA.Key,
		privileged:            privileged,
		moverSecurityContext:  source.Spec.Restic.MoverSecurityContext,
		pruneInterval:         source.Spec.Restic.PruneIntervalDays,
		retainPolicy:          source.Spec.Restic.Retain,
		sourceStatus:          source.Status.Restic,
		latestMoverStatus:     source.Status.LatestMoverStatus,
	}, nil
}

func (rb *Builder) FromDestination(client client.Client, logger logr.Logger,
	eventRecorder events.EventRecorder,
	destination *volsyncv1alpha1.ReplicationDestination, privileged bool) (mover.Mover, error) {
	// Only build if the CR belongs to us
	if destination.Spec.Restic == nil {
		return nil, nil
	}

	if destination.Status.LatestMoverStatus == nil {
		destination.Status.LatestMoverStatus = &volsyncv1alpha1.MoverStatus{}
	}

	vh, err := volumehandler.NewVolumeHandler(
		volumehandler.WithClient(client),
		volumehandler.WithRecorder(eventRecorder),
		volumehandler.WithOwner(destination),
		volumehandler.FromDestination(&destination.Spec.Restic.ReplicationDestinationVolumeOptions),
	)
	if err != nil {
		return nil, err
	}

	return &Mover{
		client:                client,
		logger:                logger.WithValues("method", "Restic"),
		eventRecorder:         eventRecorder,
		owner:                 destination,
		vh:                    vh,
		containerImage:        rb.getResticContainerImage(),
		cacheAccessModes:      destination.Spec.Restic.CacheAccessModes,
		cacheCapacity:         destination.Spec.Restic.CacheCapacity,
		cacheStorageClassName: destination.Spec.Restic.CacheStorageClassName,
		repositoryName:        destination.Spec.Restic.Repository,
		isSource:              false,
		paused:                destination.Spec.Paused,
		mainPVCName:           destination.Spec.Restic.DestinationPVC,
		caSecretName:          destination.Spec.Restic.CustomCA.SecretName,
		caSecretKey:           destination.Spec.Restic.CustomCA.Key,
		privileged:            privileged,
		moverSecurityContext:  destination.Spec.Restic.MoverSecurityContext,
		restoreAsOf:           destination.Spec.Restic.RestoreAsOf,
		previous:              destination.Spec.Restic.Previous,
		latestMoverStatus:     destination.Status.LatestMoverStatus,
	}, nil
}
