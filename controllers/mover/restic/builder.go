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
	"sigs.k8s.io/controller-runtime/pkg/client"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/controllers/mover"
	"github.com/backube/volsync/controllers/volumehandler"
)

// defaultResticContainerImage is the default container image for the restic
// data mover
const defaultResticContainerImage = "quay.io/backube/volsync-mover-restic:latest"

// resticContainerImage is the container image name of the restic data mover
var resticContainerImage string

type Builder struct{}

var _ mover.Builder = &Builder{}

func Register() {
	flag.StringVar(&resticContainerImage, "restic-container-image",
		defaultResticContainerImage, "The container image for the restic data mover")
	mover.Register(&Builder{})
}

func (rb *Builder) VersionInfo() string {
	return fmt.Sprintf("Restic container: %s", resticContainerImage)
}

func (rb *Builder) FromSource(client client.Client, logger logr.Logger,
	source *volsyncv1alpha1.ReplicationSource) (mover.Mover, error) {
	// Only build if the CR belongs to us
	if source.Spec.Restic == nil {
		return nil, nil
	}

	// Create ReplicationSourceResticStatus to write restic status
	if source.Status.Restic == nil {
		source.Status.Restic = &volsyncv1alpha1.ReplicationSourceResticStatus{}
	}

	vh, err := volumehandler.NewVolumeHandler(
		volumehandler.WithClient(client),
		volumehandler.WithOwner(source),
		volumehandler.FromSource(&source.Spec.Restic.ReplicationSourceVolumeOptions),
	)
	if err != nil {
		return nil, err
	}

	return &Mover{
		client:                client,
		logger:                logger.WithValues("method", "Restic"),
		owner:                 source,
		vh:                    vh,
		cacheAccessModes:      source.Spec.Restic.CacheAccessModes,
		cacheCapacity:         source.Spec.Restic.CacheCapacity,
		cacheStorageClassName: source.Spec.Restic.CacheStorageClassName,
		repositoryName:        source.Spec.Restic.Repository,
		isSource:              true,
		paused:                source.Spec.Paused,
		mainPVCName:           &source.Spec.SourcePVC,
		pruneInterval:         source.Spec.Restic.PruneIntervalDays,
		retainPolicy:          source.Spec.Restic.Retain,
		sourceStatus:          source.Status.Restic,
	}, nil
}

func (rb *Builder) FromDestination(client client.Client, logger logr.Logger,
	destination *volsyncv1alpha1.ReplicationDestination) (mover.Mover, error) {
	// Only build if the CR belongs to us
	if destination.Spec.Restic == nil {
		return nil, nil
	}

	vh, err := volumehandler.NewVolumeHandler(
		volumehandler.WithClient(client),
		volumehandler.WithOwner(destination),
		volumehandler.FromDestination(&destination.Spec.Restic.ReplicationDestinationVolumeOptions),
	)
	if err != nil {
		return nil, err
	}

	return &Mover{
		client:                client,
		logger:                logger.WithValues("method", "Restic"),
		owner:                 destination,
		vh:                    vh,
		cacheAccessModes:      destination.Spec.Restic.CacheAccessModes,
		cacheCapacity:         destination.Spec.Restic.CacheCapacity,
		cacheStorageClassName: destination.Spec.Restic.CacheStorageClassName,
		repositoryName:        destination.Spec.Restic.Repository,
		isSource:              false,
		paused:                destination.Spec.Paused,
		mainPVCName:           destination.Spec.Restic.DestinationPVC,
		restoreAsOf:           destination.Spec.Restic.RestoreAsOf,
		previous:              destination.Spec.Restic.Previous,
	}, nil
}
