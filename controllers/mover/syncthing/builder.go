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

	"github.com/go-logr/logr"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/controllers/mover"
)

// syncthingContainerImage is the container image name of the syncthing data mover

type Builder struct {
	viper *viper.Viper
	flags *flag.FlagSet
}

var _ mover.Builder = &Builder{}

const (
	defaultSyncthingContainerImage = "quay.io/backube/volsync-mover-syncthing:latest"
	syncthingContainerImageFlag    = "syncthing-container-image"
	syncthingContainerImageEnvVar  = "RELATED_IMAGE_SYNCTHING_CONTAINER"
)

func Register() error {
	// use global viper & command line flag
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

	// Set default syncthing container image - will be used if both command line flag and env var are not set
	b.viper.SetDefault(syncthingContainerImageFlag, defaultSyncthingContainerImage)

	// Setup command line flag for the syncthing container image
	b.flags.String(syncthingContainerImageFlag, defaultSyncthingContainerImage,
		"The container image for the syncthing data mover")
	// Viper will check for command line flag first, then fallback to the env var
	err := b.viper.BindEnv(syncthingContainerImageFlag, syncthingContainerImageEnvVar)

	return b, err
}

func (rb *Builder) VersionInfo() string {
	return fmt.Sprintf("Syncthing container: %s", rb.getSyncthingContainerImage())
}

func (rb *Builder) getSyncthingContainerImage() string {
	return rb.viper.GetString(syncthingContainerImageFlag)
}

func (rb *Builder) FromSource(client client.Client, logger logr.Logger,
	eventRecorder events.EventRecorder,
	source *volsyncv1alpha1.ReplicationSource) (mover.Mover, error) {
	// Only build if the CR belongs to us
	if source.Spec.Syncthing == nil {
		return nil, nil
	}

	// Create ReplicationSourceSyncthingStatus to write syncthing status
	if source.Status.Syncthing == nil {
		source.Status.Syncthing = &volsyncv1alpha1.ReplicationSourceSyncthingStatus{}
	}

	// servicetype or default
	var serviceType corev1.ServiceType
	if source.Spec.Syncthing.ServiceType != nil {
		serviceType = *source.Spec.Syncthing.ServiceType
	} else {
		serviceType = corev1.ServiceTypeLoadBalancer
	}

	syncthingLogger := logger.WithValues("method", "Syncthing")

	return &Mover{
		client:             client,
		logger:             syncthingLogger,
		owner:              source,
		eventRecorder:      eventRecorder,
		configStorageClass: source.Spec.Syncthing.ConfigStorageClassName,
		configAccessModes:  source.Spec.Syncthing.ConfigAccessModes,
		containerImage:     rb.getSyncthingContainerImage(),
		peerList:           source.Spec.Syncthing.Peers,
		paused:             source.Spec.Paused,
		dataPVCName:        &source.Spec.SourcePVC,
		status:             source.Status.Syncthing,
		serviceType:        serviceType,
		syncthing: Syncthing{
			APIConfig: &APIConfig{
				APIURL: "",
			},
			// all logs from Syncthing's logger should be V(4)
			logger: syncthingLogger.WithValues("struct", "Syncthing"),
		},
		// defer setting the VolumeHandler
	}, nil
}

// syncthing is destination-only
func (rb *Builder) FromDestination(client client.Client, logger logr.Logger,
	eventRecorder events.EventRecorder,
	destination *volsyncv1alpha1.ReplicationDestination) (mover.Mover, error) {
	return nil, nil
}
