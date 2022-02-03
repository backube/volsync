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

package syncthing

import (
	"flag"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/controllers/mover"
)

// defaultSyncthingContainerImage is the default container image for the syncthing
// data mover
const defaultSyncthingContainerImage = "quay.io/backube/volsync-mover-syncthing:latest"

// syncthingContainerImage is the container image name of the syncthing data mover
var syncthingContainerImage string

type Builder struct{}

var _ mover.Builder = &Builder{}

func Register() {
	flag.StringVar(&syncthingContainerImage, "syncthing-container-image",
		defaultSyncthingContainerImage, "The container image for the syncthing data mover")
	mover.Register(&Builder{})
}

func (rb *Builder) VersionInfo() string {
	return fmt.Sprintf("Syncthing container: %s", syncthingContainerImage)
}

func (rb *Builder) FromSource(client client.Client, logger logr.Logger,
	source *volsyncv1alpha1.ReplicationSource) (mover.Mover, error) {
	// Only build if the CR belongs to us
	if source.Spec.Syncthing == nil {
		return nil, nil
	}

	// Create ReplicationSourceSyncthingStatus to write syncthing status
	if source.Status.Syncthing == nil {
		source.Status.Syncthing = &volsyncv1alpha1.ReplicationSourceSyncthingStatus{}
	}

	// temporary solution
	// set the apiURL to localhost if running in local environment
	var apiURL = ""
	if os.Getenv("RUN_LOCAL") == "true" {
		apiURL = "http://127.0.0.1:8384"
	}

	return &Mover{
		client:      client,
		logger:      logger.WithValues("method", "Syncthing"),
		owner:       source,
		peerList:    source.Spec.Syncthing.Peers,
		isSource:    true,
		paused:      source.Spec.Paused,
		dataPVCName: &source.Spec.SourcePVC,
		status:      source.Status.Syncthing,
		apiKey:      "",
		apiURL:      apiURL,
	}, nil
}

// not implemented for syncthing
func (rb *Builder) FromDestination(client client.Client, logger logr.Logger,
	destination *volsyncv1alpha1.ReplicationDestination) (mover.Mover, error) {
	return nil, fmt.Errorf("syncthing mover not implemented for destination")
}
