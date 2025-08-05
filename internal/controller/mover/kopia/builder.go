//go:build !disable_kopia

/*
Copyright 2024 The VolSync authors.

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

package kopia

import (
	"flag"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/spf13/viper"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/internal/controller/mover"
	"github.com/backube/volsync/internal/controller/utils"
	"github.com/backube/volsync/internal/controller/volumehandler"
)

const (
	kopiaMoverName = "kopia"
	// defaultKopiaContainerImage is the default container image for the kopia
	// data mover
	defaultKopiaContainerImage = "quay.io/backube/volsync:latest"
	// Command line flag will be checked first
	// If command line flag not set, the RELATED_IMAGE_ env var will be used
	kopiaContainerImageFlag   = "kopia-container-image"
	kopiaContainerImageEnvVar = "RELATED_IMAGE_KOPIA_CONTAINER"
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

	// Set default kopia container image - will be used if both command line flag and env var are not set
	b.viper.SetDefault(kopiaContainerImageFlag, defaultKopiaContainerImage)

	// Setup command line flag for the kopia container image
	b.flags.String(kopiaContainerImageFlag, defaultKopiaContainerImage,
		"The container image for the kopia data mover")
	// Viper will check for command line flag first, then fallback to the env var
	err := b.viper.BindEnv(kopiaContainerImageFlag, kopiaContainerImageEnvVar)

	return b, err
}

func (kb *Builder) Name() string { return kopiaMoverName }

func (kb *Builder) VersionInfo() string {
	return fmt.Sprintf("Kopia container: %s", kb.getKopiaContainerImage())
}

// kopiaContainerImage is the container image name of the kopia data mover
func (kb *Builder) getKopiaContainerImage() string {
	return kb.viper.GetString(kopiaContainerImageFlag)
}

func (kb *Builder) FromSource(client client.Client, logger logr.Logger,
	eventRecorder events.EventRecorder,
	source *volsyncv1alpha1.ReplicationSource, privileged bool) (mover.Mover, error) {
	// Only build if the CR belongs to us
	if source.Spec.Kopia == nil {
		return nil, nil
	}

	// Initialize status fields
	kb.initializeSourceStatus(source)

	// Create volume handler
	vh, err := kb.createVolumeHandlerForSource(client, eventRecorder, source)
	if err != nil {
		return nil, err
	}

	// Create and return the mover
	return kb.createSourceMover(client, logger, eventRecorder, source, vh, privileged)
}

// initializeSourceStatus initializes the status fields for the ReplicationSource
func (kb *Builder) initializeSourceStatus(source *volsyncv1alpha1.ReplicationSource) {
	// Create ReplicationSourceKopiaStatus to write kopia status
	if source.Status.Kopia == nil {
		source.Status.Kopia = &volsyncv1alpha1.ReplicationSourceKopiaStatus{}
	}

	if source.Status.LatestMoverStatus == nil {
		source.Status.LatestMoverStatus = &volsyncv1alpha1.MoverStatus{}
	}
}

// createVolumeHandlerForSource creates a volume handler for the ReplicationSource
func (kb *Builder) createVolumeHandlerForSource(client client.Client,
	eventRecorder events.EventRecorder,
	source *volsyncv1alpha1.ReplicationSource) (*volumehandler.VolumeHandler, error) {
	return volumehandler.NewVolumeHandler(
		volumehandler.WithClient(client),
		volumehandler.WithRecorder(eventRecorder),
		volumehandler.WithOwner(source),
		volumehandler.FromSource(&source.Spec.Kopia.ReplicationSourceVolumeOptions),
	)
}

// createSourceMover creates the Mover instance for a ReplicationSource
func (kb *Builder) createSourceMover(client client.Client, logger logr.Logger,
	eventRecorder events.EventRecorder, source *volsyncv1alpha1.ReplicationSource,
	vh *volumehandler.VolumeHandler, privileged bool) (*Mover, error) {
	isSource := true

	// Generate username and hostname for multi-tenancy
	username := generateUsername(source.Spec.Kopia.Username)
	hostname := generateHostname(source.Spec.Kopia.Hostname, source.GetNamespace(), source.GetName())

	saHandler := utils.NewSAHandler(client, source, isSource, privileged,
		source.Spec.Kopia.MoverServiceAccount)

	// Initialize metrics
	metrics := newKopiaMetrics()

	return &Mover{
		client:                client,
		logger:                logger.WithValues("method", "Kopia"),
		eventRecorder:         eventRecorder,
		owner:                 source,
		vh:                    vh,
		saHandler:             saHandler,
		containerImage:        kb.getKopiaContainerImage(),
		cacheAccessModes:      source.Spec.Kopia.CacheAccessModes,
		cacheCapacity:         source.Spec.Kopia.CacheCapacity,
		cacheStorageClassName: source.Spec.Kopia.CacheStorageClassName,
		repositoryName:        source.Spec.Kopia.Repository,
		isSource:              isSource,
		paused:                source.Spec.Paused,
		mainPVCName:           &source.Spec.SourcePVC,
		customCASpec:          volsyncv1alpha1.CustomCASpec(source.Spec.Kopia.CustomCA),
		policyConfig:          source.Spec.Kopia.PolicyConfig,
		privileged:            privileged,
		metrics:               metrics,
		username:              username,
		hostname:              hostname,
		sourcePathOverride:    source.Spec.Kopia.SourcePathOverride,
		maintenanceInterval:   source.Spec.Kopia.MaintenanceIntervalDays,
		retainPolicy:          source.Spec.Kopia.Retain,
		compression:           source.Spec.Kopia.Compression,
		parallelism:           source.Spec.Kopia.Parallelism,
		actions:               source.Spec.Kopia.Actions,
		sourceStatus:          source.Status.Kopia,
		latestMoverStatus:     source.Status.LatestMoverStatus,
		moverConfig:           source.Spec.Kopia.MoverConfig,
	}, nil
}

func (kb *Builder) FromDestination(client client.Client, logger logr.Logger,
	eventRecorder events.EventRecorder,
	destination *volsyncv1alpha1.ReplicationDestination, privileged bool) (mover.Mover, error) {
	// Only build if the CR belongs to us
	if destination.Spec.Kopia == nil {
		return nil, nil
	}

	if destination.Status.LatestMoverStatus == nil {
		destination.Status.LatestMoverStatus = &volsyncv1alpha1.MoverStatus{}
	}

	vh, err := volumehandler.NewVolumeHandler(
		volumehandler.WithClient(client),
		volumehandler.WithRecorder(eventRecorder),
		volumehandler.WithOwner(destination),
		volumehandler.FromDestination(&destination.Spec.Kopia.ReplicationDestinationVolumeOptions),
	)
	if err != nil {
		return nil, err
	}

	isSource := false

	// Generate username and hostname for multi-tenancy
	username := generateUsername(destination.Spec.Kopia.Username)
	hostname := generateHostname(destination.Spec.Kopia.Hostname, destination.GetNamespace(), destination.GetName())

	saHandler := utils.NewSAHandler(client, destination, isSource, privileged,
		destination.Spec.Kopia.MoverServiceAccount)

	// Initialize metrics
	metrics := newKopiaMetrics()

	return &Mover{
		client:                client,
		logger:                logger.WithValues("method", "Kopia"),
		eventRecorder:         eventRecorder,
		owner:                 destination,
		vh:                    vh,
		saHandler:             saHandler,
		containerImage:        kb.getKopiaContainerImage(),
		cacheAccessModes:      destination.Spec.Kopia.CacheAccessModes,
		cacheCapacity:         destination.Spec.Kopia.CacheCapacity,
		cacheStorageClassName: destination.Spec.Kopia.CacheStorageClassName,
		cleanupCachePVC:       destination.Spec.Kopia.CleanupCachePVC,
		repositoryName:        destination.Spec.Kopia.Repository,
		isSource:              isSource,
		paused:                destination.Spec.Paused,
		mainPVCName:           destination.Spec.Kopia.DestinationPVC,
		cleanupTempPVC:        destination.Spec.Kopia.CleanupTempPVC,
		customCASpec:          volsyncv1alpha1.CustomCASpec(destination.Spec.Kopia.CustomCA),
		policyConfig:          destination.Spec.Kopia.PolicyConfig,
		privileged:            privileged,
		metrics:               metrics,
		username:              username,
		hostname:              hostname,
		restoreAsOf:           destination.Spec.Kopia.RestoreAsOf,
		shallow:               destination.Spec.Kopia.Shallow,
		latestMoverStatus:     destination.Status.LatestMoverStatus,
		moverConfig:           destination.Spec.Kopia.MoverConfig,
	}, nil
}

// generateUsername returns the username for Kopia identity
// If specified, uses the provided username, otherwise defaults to "volsync"
func generateUsername(username *string) string {
	if username != nil && *username != "" {
		return *username
	}
	return "volsync"
}

// generateHostname returns the hostname for Kopia identity
// If specified, uses the provided hostname, otherwise defaults to "<namespace>-<name>"
func generateHostname(hostname *string, namespace, name string) string {
	if hostname != nil && *hostname != "" {
		return *hostname
	}

	// Generate default hostname from namespace and resource name
	// Replace any characters that aren't allowed in Kopia hostnames
	defaultHostname := fmt.Sprintf("%s-%s", namespace, name)
	// Replace underscores and other invalid characters with hyphens
	defaultHostname = strings.ReplaceAll(defaultHostname, "_", "-")
	// Remove any invalid characters (only allow alphanumeric, dots, and hyphens)
	validHostname := ""
	for _, r := range defaultHostname {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' {
			validHostname += string(r)
		}
	}

	// Ensure hostname doesn't start or end with a hyphen or dot
	validHostname = strings.Trim(validHostname, "-.")

	if validHostname == "" {
		return "volsync-default"
	}

	return validHostname
}
