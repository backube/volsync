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
	// defaultUsername is the fallback username when sanitization results in empty string
	defaultUsername = "volsync-default"
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
	return kb.createSourceMover(client, logger, eventRecorder, source, vh, privileged), nil
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
	vh *volumehandler.VolumeHandler, privileged bool) *Mover {
	isSource := true

	// Generate username and hostname for multi-tenancy
	username := generateUsername(source.Spec.Kopia.Username, source.GetName(), source.GetNamespace())
	hostname := generateHostname(source.Spec.Kopia.Hostname, &source.Spec.SourcePVC,
		source.GetNamespace(), source.GetName())

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
	}
}

//nolint:funlen
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
	// Priority order:
	// 1. Explicit username/hostname fields (highest priority)
	// 2. sourceIdentity helper (generates from source info)
	// 3. Default generation from destination name/namespace
	var username, hostname string

	// Check if sourceIdentity helper is provided and should be used
	useSourceIdentity := destination.Spec.Kopia.SourceIdentity != nil &&
		destination.Spec.Kopia.SourceIdentity.SourceName != "" &&
		destination.Spec.Kopia.SourceIdentity.SourceNamespace != ""

	// Generate username with proper priority
	if destination.Spec.Kopia.Username != nil && *destination.Spec.Kopia.Username != "" {
		// Explicit username has highest priority
		username = *destination.Spec.Kopia.Username
	} else if useSourceIdentity {
		// Use sourceIdentity to generate username
		si := destination.Spec.Kopia.SourceIdentity
		username = generateUsername(nil, si.SourceName, si.SourceNamespace)
	} else {
		// Default generation from destination
		username = generateUsername(nil, destination.GetName(), destination.GetNamespace())
	}

	// Generate hostname with proper priority
	if destination.Spec.Kopia.Hostname != nil && *destination.Spec.Kopia.Hostname != "" {
		// Explicit hostname has highest priority
		hostname = *destination.Spec.Kopia.Hostname
	} else if useSourceIdentity {
		// Use sourceIdentity to generate hostname
		si := destination.Spec.Kopia.SourceIdentity
		hostname = generateHostname(nil, destination.Spec.Kopia.DestinationPVC,
			si.SourceNamespace, si.SourceName)
	} else {
		// Default generation from destination
		hostname = generateHostname(nil, destination.Spec.Kopia.DestinationPVC,
			destination.GetNamespace(), destination.GetName())
	}

	saHandler := utils.NewSAHandler(client, destination, isSource, privileged,
		destination.Spec.Kopia.MoverServiceAccount)

	// Initialize metrics
	metrics := newKopiaMetrics()

	// Initialize Kopia status if not already present
	if destination.Status.Kopia == nil {
		destination.Status.Kopia = &volsyncv1alpha1.ReplicationDestinationKopiaStatus{}
	}

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
		previous:              destination.Spec.Kopia.Previous,
		destinationStatus:     destination.Status.Kopia,
		latestMoverStatus:     destination.Status.LatestMoverStatus,
		moverConfig:           destination.Spec.Kopia.MoverConfig,
	}, nil
}

// generateUsername returns the username for Kopia identity
// Priority order:
// 1. If specified, uses the provided username as-is (highest priority)
// 2. Sanitizes object name and appends namespace if combined length ≤ 50 chars
// 3. Uses sanitized object name only if combined length > 50 chars
// 4. Returns "volsync-default" if sanitized username is empty
// Username rules: alphanumeric, hyphens, and underscores allowed
// (More permissive than hostnames to maintain backward compatibility)
func generateUsername(username *string, objectName string, namespace string) string {
	if username != nil && *username != "" {
		return *username
	}

	// Sanitize the object name for username
	// Username rules: alphanumeric, hyphens, and underscores allowed
	// (More permissive than hostnames to maintain backward compatibility)
	validObjectName := ""
	for _, r := range objectName {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			validObjectName += string(r)
		}
	}

	// Ensure object name doesn't start or end with a hyphen or underscore
	validObjectName = strings.Trim(validObjectName, "-_")

	if validObjectName == "" {
		return defaultUsername
	}

	// Try to append namespace if there's room and namespace is valid
	// Username approach: object-name-namespace for better identification
	validNamespace := ""
	for _, r := range namespace {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			validNamespace += string(r)
		}
	}
	validNamespace = strings.Trim(validNamespace, "-_")

	// Kopia usernames have practical length limits (typically ~50 chars is reasonable)
	// If we can fit namespace with a separator, include it for better multi-tenancy
	const maxUsernameLength = 50 // Maximum reasonable length for Kopia usernames
	if validNamespace != "" {
		candidateUsername := validObjectName + "-" + validNamespace
		if len(candidateUsername) <= maxUsernameLength {
			return candidateUsername
		}
	}

	return validObjectName
}

// generateHostname returns the hostname for Kopia identity with namespace-first priority for better multi-tenancy
// Priority order:
// 1. Custom hostname (if provided) - highest priority, used as-is
// 2. Namespace-based hostname - use namespace as the base
// 3. Append PVC name if space allows - try to add PVC name if combined length is reasonable (≤50 characters)
// 4. Fallback patterns:
//   - If namespace + PVC too long, use just namespace
//   - If namespace is empty/invalid, use "namespace-name" format as final fallback
//
// 5. Sanitization - alphanumeric, dots, hyphens; convert underscores
func generateHostname(hostname *string, pvcName *string, namespace, name string) string {
	if hostname != nil && *hostname != "" {
		return *hostname
	}

	// Sanitize namespace for hostname use
	sanitizedNamespace := sanitizeForHostname(namespace)

	// Sanitize PVC name for hostname use
	var sanitizedPVC string
	if pvcName != nil && *pvcName != "" {
		sanitizedPVC = sanitizeForHostname(*pvcName)
	}

	const maxHostnameLength = 50 // Maximum reasonable length for Kopia hostnames
	var defaultHostname string

	// Priority 1: Use namespace as base (namespace-first approach)
	if sanitizedNamespace != "" {
		if sanitizedPVC != "" {
			// Try to combine namespace + PVC if it fits within reasonable length
			candidateHostname := sanitizedNamespace + "-" + sanitizedPVC
			if len(candidateHostname) <= maxHostnameLength {
				defaultHostname = candidateHostname
			} else {
				// If combined is too long, use just namespace for better multi-tenancy
				defaultHostname = sanitizedNamespace
			}
		} else {
			// No PVC name, use just namespace
			defaultHostname = sanitizedNamespace
		}
	} else {
		// Priority 2: Fallback to traditional namespace-name pattern if namespace is invalid
		fallbackHostname := fmt.Sprintf("%s-%s", namespace, name)
		defaultHostname = sanitizeForHostname(fallbackHostname)

		// If the fallback also results in empty string, just use the sanitized object name
		if defaultHostname == "" {
			defaultHostname = sanitizeForHostname(name)
		}
	}

	if defaultHostname == "" {
		// Log when fallback is used for troubleshooting
		// Note: In production, consider adding proper logging with context
		return defaultUsername
	}

	return defaultHostname
}

// sanitizeForHostname sanitizes a string for use as a hostname
// Hostname rules: alphanumeric, dots, and hyphens only
// Replaces underscores with hyphens per hostname standards
// Replaces underscores with hyphens, removes invalid characters, and trims separators
func sanitizeForHostname(input string) string {
	// Replace underscores with hyphens
	sanitized := strings.ReplaceAll(input, "_", "-")

	// Remove any invalid characters (only allow alphanumeric, dots, and hyphens)
	validChars := ""
	for _, r := range sanitized {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' {
			validChars += string(r)
		}
	}

	// Ensure hostname doesn't start or end with a hyphen or dot
	return strings.Trim(validChars, "-.")
}
