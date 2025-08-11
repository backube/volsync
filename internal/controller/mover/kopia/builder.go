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
	"context"
	"flag"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/spf13/viper"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
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
	// maxUsernameLength is the maximum reasonable length for Kopia usernames
	maxUsernameLength = 50
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

func newBuilder(viperInstance *viper.Viper, flags *flag.FlagSet) (*Builder, error) {
	// Use provided viper instance or create a new one if nil
	if viperInstance == nil {
		viperInstance = viper.New()
	}

	// Use provided flags or create a new FlagSet if nil
	if flags == nil {
		flags = flag.NewFlagSet("kopia", flag.ContinueOnError)
	}

	b := &Builder{
		viper: viperInstance,
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
	// Initialize Status if it's nil
	if source.Status == nil {
		source.Status = &volsyncv1alpha1.ReplicationSourceStatus{}
	}

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
		repositoryPVC:         source.Spec.Kopia.RepositoryPVC,
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

	// Validate that user has provided identity information for ReplicationDestination
	if err := kb.validateDestinationIdentity(destination); err != nil {
		logger.Error(err, "Invalid ReplicationDestination configuration",
			"destination", destination.GetName(),
			"namespace", destination.GetNamespace(),
			"hint", "Please provide either sourceIdentity OR both username and hostname")
		return nil, err
	}

	// Initialize Status if it's nil
	if destination.Status == nil {
		destination.Status = &volsyncv1alpha1.ReplicationDestinationStatus{}
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
	var sourcePathOverride *string
	var repositoryName string

	// Check if sourceIdentity helper is provided and should be used
	useSourceIdentity := destination.Spec.Kopia.SourceIdentity != nil &&
		destination.Spec.Kopia.SourceIdentity.SourceName != ""

	// Default sourceNamespace to the destination's namespace if not provided
	var sourceNamespace string
	if useSourceIdentity {
		if destination.Spec.Kopia.SourceIdentity.SourceNamespace != "" {
			sourceNamespace = destination.Spec.Kopia.SourceIdentity.SourceNamespace
		} else {
			sourceNamespace = destination.GetNamespace()
			logger.V(1).Info("Using destination namespace as sourceNamespace",
				"namespace", sourceNamespace)
		}
	}

	// Generate username with proper priority
	if destination.Spec.Kopia.Username != nil && *destination.Spec.Kopia.Username != "" {
		// Explicit username has highest priority
		username = *destination.Spec.Kopia.Username
	} else if useSourceIdentity {
		// Use sourceIdentity to generate username
		si := destination.Spec.Kopia.SourceIdentity
		username = generateUsername(nil, si.SourceName, sourceNamespace)
	} else {
		// Default generation from destination
		username = generateUsername(nil, destination.GetName(), destination.GetNamespace())
	}

	// Generate hostname and discover sourcePathOverride with proper priority
	if destination.Spec.Kopia.Hostname != nil && *destination.Spec.Kopia.Hostname != "" {
		// Explicit hostname has highest priority
		hostname = *destination.Spec.Kopia.Hostname
	} else if useSourceIdentity {
		// Use sourceIdentity to generate hostname and discover sourcePathOverride
		si := destination.Spec.Kopia.SourceIdentity
		var pvcNameToUse *string

		// Priority for sourcePathOverride:
		// 1. Explicitly provided sourcePathOverride in sourceIdentity
		// 2. Auto-discovered from ReplicationSource
		if si.SourcePathOverride != nil {
			sourcePathOverride = si.SourcePathOverride
			logger.V(1).Info("Using explicitly provided sourcePathOverride",
				"sourcePathOverride", *sourcePathOverride)
		}

		// Priority for PVC name:
		// 1. Explicitly provided sourcePVCName
		// 2. Auto-discovered from ReplicationSource
		// 3. Fallback to destination PVC
		if si.SourcePVCName != "" {
			pvcNameToUse = &si.SourcePVCName
		} else {
			// Try to auto-discover the source info from the ReplicationSource
			discoveredInfo := kb.discoverSourceInfo(client, si.SourceName, sourceNamespace, logger)
			if discoveredInfo.pvcName != "" {
				pvcNameToUse = &discoveredInfo.pvcName
				logger.V(1).Info("Auto-discovered source PVC from ReplicationSource",
					"sourceName", si.SourceName,
					"sourceNamespace", sourceNamespace,
					"discoveredPVC", discoveredInfo.pvcName)
			} else {
				// Fallback to destination PVC if source PVC name not provided or discovered
				pvcNameToUse = destination.Spec.Kopia.DestinationPVC
				logger.V(1).Info("Could not discover source PVC, using destination PVC for hostname",
					"destinationPVC", destination.Spec.Kopia.DestinationPVC)
			}

			// Use discovered sourcePathOverride if not explicitly provided
			if sourcePathOverride == nil && discoveredInfo.sourcePathOverride != nil {
				sourcePathOverride = discoveredInfo.sourcePathOverride
				logger.V(1).Info("Auto-discovered sourcePathOverride from ReplicationSource",
					"sourceName", si.SourceName,
					"sourceNamespace", sourceNamespace,
					"sourcePathOverride", *sourcePathOverride)
			}

			// Use discovered repository if destination repository is empty
			if destination.Spec.Kopia.Repository == "" && discoveredInfo.repository != "" {
				repositoryName = discoveredInfo.repository
				logger.V(1).Info("Auto-discovered repository from ReplicationSource",
					"sourceName", si.SourceName,
					"sourceNamespace", sourceNamespace,
					"repository", repositoryName)
			}
		}
		hostname = generateHostname(nil, pvcNameToUse,
			sourceNamespace, si.SourceName)
	} else {
		// Default generation from destination
		hostname = generateHostname(nil, destination.Spec.Kopia.DestinationPVC,
			destination.GetNamespace(), destination.GetName())
	}

	// Set repository name - prioritize explicit destination repository over discovered one
	if destination.Spec.Kopia.Repository != "" {
		repositoryName = destination.Spec.Kopia.Repository
	}
	// If repositoryName is still empty at this point, it means:
	// 1. Destination repository is empty AND
	// 2. Either sourceIdentity is not used OR no repository was discovered
	// In this case, repositoryName will remain empty string, which is the existing behavior

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
		repositoryName:        repositoryName,
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
		sourcePathOverride:    sourcePathOverride,
		restoreAsOf:           destination.Spec.Kopia.RestoreAsOf,
		shallow:               destination.Spec.Kopia.Shallow,
		previous:              destination.Spec.Kopia.Previous,
		destinationStatus:     destination.Status.Kopia,
		latestMoverStatus:     destination.Status.LatestMoverStatus,
		moverConfig:           destination.Spec.Kopia.MoverConfig,
	}, nil
}

// sanitizeForIdentifier sanitizes a string for use as username or hostname component
// allowUnderscore: true for usernames (backward compatibility), false for hostnames
// allowDots: true for hostnames, false for usernames
// Returns sanitized string with leading/trailing separators removed
func sanitizeForIdentifier(input string, allowUnderscore, allowDots bool) string {
	var validChars strings.Builder
	for _, r := range input {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			validChars.WriteRune(r)
		} else if allowUnderscore && r == '_' {
			validChars.WriteRune(r)
		} else if !allowUnderscore && r == '_' {
			// Convert underscores to hyphens for hostnames
			validChars.WriteRune('-')
		} else if allowDots && r == '.' {
			// Dots are allowed in hostnames
			validChars.WriteRune(r)
		}
	}
	
	// Trim separators based on what's allowed
	trimChars := "-"
	if allowUnderscore {
		trimChars = "-_"
	}
	if allowDots {
		trimChars = trimChars + "."
	}
	return strings.Trim(validChars.String(), trimChars)
}

// generateUsername returns the username for Kopia identity
// Priority order:
// 1. If specified, uses the provided username as-is (highest priority)
// 2. Sanitizes object name and appends namespace if combined length ≤ maxUsernameLength
// 3. Uses sanitized object name only if combined length > maxUsernameLength
// 4. Returns "volsync-default" if sanitized username is empty
// Username rules: alphanumeric, hyphens, and underscores allowed
// (More permissive than hostnames to maintain backward compatibility)
func generateUsername(username *string, objectName string, namespace string) string {
	if username != nil && *username != "" {
		return *username
	}

	// Sanitize the object name for username (allows underscores, no dots)
	validObjectName := sanitizeForIdentifier(objectName, true, false)
	if validObjectName == "" {
		return defaultUsername
	}

	// Try to append namespace if there's room and namespace is valid
	// Username approach: object-name-namespace for better identification
	validNamespace := sanitizeForIdentifier(namespace, true, false)

	// Kopia usernames have practical length limits
	// If we can fit namespace with a separator, include it for better multi-tenancy
	if validNamespace != "" {
		candidateUsername := validObjectName + "-" + validNamespace
		if len(candidateUsername) <= maxUsernameLength {
			return candidateUsername
		}
	}

	// Ensure the username doesn't exceed the maximum length
	if len(validObjectName) > maxUsernameLength {
		return validObjectName[:maxUsernameLength]
	}
	return validObjectName
}

// generateHostname returns the hostname for Kopia identity based on namespace for multi-tenancy
// Priority order:
// 1. Custom hostname (if provided) - highest priority, used as-is
// 2. Namespace-based hostname - always use namespace only (no PVC name)
// 3. Fallback patterns:
//   - If namespace is empty/invalid, use "namespace-name" format as fallback
//   - If that fails, use object name
//   - Final fallback: defaultUsername
//
// Sanitization: alphanumeric, dots, hyphens; convert underscores to hyphens
func generateHostname(hostname *string, _ *string, namespace, name string) string {
	if hostname != nil && *hostname != "" {
		return *hostname
	}

	// Sanitize namespace and name for hostname use
	sanitizedNamespace := sanitizeForHostname(namespace)
	sanitizedName := sanitizeForHostname(name)

	// Generate hostname with namespace-objectname pattern for uniqueness
	var defaultHostname string
	if sanitizedNamespace != "" && sanitizedName != "" {
		// Use namespace-objectname for uniqueness within namespace
		defaultHostname = sanitizedNamespace + "-" + sanitizedName
	} else if sanitizedNamespace != "" {
		// Use namespace only if name is invalid
		defaultHostname = sanitizedNamespace
	} else if sanitizedName != "" {
		// Use name only if namespace is invalid
		defaultHostname = sanitizedName
	} else {
		// Fallback to traditional namespace-name pattern if both are problematic
		fallbackHostname := fmt.Sprintf("%s-%s", namespace, name)
		defaultHostname = sanitizeForHostname(fallbackHostname)
	}

	if defaultHostname == "" {
		// Final fallback to defaultUsername
		return defaultUsername
	}

	return defaultHostname
}

// sanitizeForHostname sanitizes a string for use as a hostname
// Hostname rules: alphanumeric, dots, and hyphens only
// Replaces underscores with hyphens per hostname standards
func sanitizeForHostname(input string) string {
	// Use common sanitization function (no underscores, allow dots)
	return sanitizeForIdentifier(input, false, true)
}

// sourceDiscoveryInfo holds discovered information from a ReplicationSource
type sourceDiscoveryInfo struct {
	pvcName            string
	sourcePathOverride *string
	repository         string
}

// discoverSourceInfo attempts to discover the source PVC name, sourcePathOverride, and repository from a
// ReplicationSource in the specified namespace. This enables automatic hostname generation and repository
// configuration matching the source's identity without requiring manual configuration.
// Returns a sourceDiscoveryInfo struct with discovered values.
func (kb *Builder) discoverSourceInfo(c client.Client, sourceName, sourceNamespace string,
	logger logr.Logger) sourceDiscoveryInfo {
	info := sourceDiscoveryInfo{}
	if sourceName == "" || sourceNamespace == "" {
		logger.V(2).Info("Cannot discover source info: missing source name or namespace",
			"sourceName", sourceName,
			"sourceNamespace", sourceNamespace)
		return info
	}

	source, err := kb.fetchReplicationSource(c, sourceName, sourceNamespace, logger)
	if err != nil || source == nil {
		return info
	}

	// Check if this is a Kopia ReplicationSource
	if source.Spec.Kopia == nil {
		logger.V(2).Info("ReplicationSource does not use Kopia mover",
			"sourceName", sourceName,
			"sourceNamespace", sourceNamespace)
		return info
	}

	return kb.extractSourceInfo(source, sourceName, sourceNamespace, logger)
}

// fetchReplicationSource retrieves a ReplicationSource from the cluster
func (kb *Builder) fetchReplicationSource(c client.Client, sourceName, sourceNamespace string,
	logger logr.Logger) (*volsyncv1alpha1.ReplicationSource, error) {
	source := &volsyncv1alpha1.ReplicationSource{}
	namespacedName := types.NamespacedName{
		Name:      sourceName,
		Namespace: sourceNamespace,
	}

	ctx := context.Background()
	err := c.Get(ctx, namespacedName, source)
	if err != nil {
		kb.logDiscoveryError(err, sourceName, sourceNamespace, logger)
		return nil, err
	}
	return source, nil
}

// logDiscoveryError logs appropriate error messages based on error type
func (kb *Builder) logDiscoveryError(err error, sourceName, sourceNamespace string, logger logr.Logger) {
	if kerrors.IsNotFound(err) {
		logger.V(2).Info("ReplicationSource not found for auto-discovery",
			"sourceName", sourceName,
			"sourceNamespace", sourceNamespace)
	} else if kerrors.IsForbidden(err) {
		logger.V(2).Info("Permission denied accessing ReplicationSource for auto-discovery",
			"sourceName", sourceName,
			"sourceNamespace", sourceNamespace,
			"error", err)
	} else {
		logger.V(2).Info("Failed to get ReplicationSource for auto-discovery",
			"sourceName", sourceName,
			"sourceNamespace", sourceNamespace,
			"error", err)
	}
}

// extractSourceInfo extracts discovery information from a ReplicationSource
func (kb *Builder) extractSourceInfo(source *volsyncv1alpha1.ReplicationSource,
	sourceName, sourceNamespace string, logger logr.Logger) sourceDiscoveryInfo {
	info := sourceDiscoveryInfo{}

	// Discover the source PVC name
	if source.Spec.SourcePVC != "" {
		info.pvcName = source.Spec.SourcePVC
		logger.V(1).Info("Successfully discovered source PVC from ReplicationSource",
			"sourceName", sourceName,
			"sourceNamespace", sourceNamespace,
			"sourcePVC", source.Spec.SourcePVC)
	} else {
		logger.V(2).Info("ReplicationSource has no source PVC configured",
			"sourceName", sourceName,
			"sourceNamespace", sourceNamespace)
	}

	// Discover the sourcePathOverride
	if source.Spec.Kopia.SourcePathOverride != nil {
		info.sourcePathOverride = source.Spec.Kopia.SourcePathOverride
		logger.V(1).Info("Successfully discovered sourcePathOverride from ReplicationSource",
			"sourceName", sourceName,
			"sourceNamespace", sourceNamespace,
			"sourcePathOverride", *source.Spec.Kopia.SourcePathOverride)
	}

	// Discover the repository
	if source.Spec.Kopia.Repository != "" {
		info.repository = source.Spec.Kopia.Repository
		logger.V(1).Info("Successfully discovered repository from ReplicationSource",
			"sourceName", sourceName,
			"sourceNamespace", sourceNamespace,
			"repository", source.Spec.Kopia.Repository)
	}

	return info
}

// discoverSourcePVC attempts to discover the source PVC name from a ReplicationSource
// in the specified namespace. This enables automatic hostname generation matching
// the source's identity without requiring manual configuration.
// Returns the source PVC name if found, empty string otherwise.
// Deprecated: Use discoverSourceInfo instead for full discovery capabilities
func (kb *Builder) discoverSourcePVC(c client.Client, sourceName, sourceNamespace string, logger logr.Logger) string {
	info := kb.discoverSourceInfo(c, sourceName, sourceNamespace, logger)
	return info.pvcName
}

// validateDestinationIdentity validates that the user has provided sufficient identity information
// for the ReplicationDestination. Users must provide either:
// 1. Both explicit username AND hostname fields, OR
// 2. A sourceIdentity field with at least sourceName
// This validation is required because we cannot automatically determine the source identity
// when restoring, as we don't know the source's identity (username and hostname combination).
func (kb *Builder) validateDestinationIdentity(destination *volsyncv1alpha1.ReplicationDestination) error {
	if destination.Spec.Kopia == nil {
		return nil
	}

	kopiaSpec := destination.Spec.Kopia

	// Check if explicit username and hostname are provided
	hasExplicitUsername := kopiaSpec.Username != nil && *kopiaSpec.Username != ""
	hasExplicitHostname := kopiaSpec.Hostname != nil && *kopiaSpec.Hostname != ""

	// Check if sourceIdentity is provided with at least sourceName
	hasSourceIdentity := kopiaSpec.SourceIdentity != nil && kopiaSpec.SourceIdentity.SourceName != ""

	// Valid if either both username/hostname are provided OR sourceIdentity is provided
	if (hasExplicitUsername && hasExplicitHostname) || hasSourceIdentity {
		return nil
	}

	// Build concise error message
	var errorMsg string
	if hasExplicitUsername && !hasExplicitHostname {
		errorMsg = "missing 'hostname' (you provided 'username' but both are required)"
	} else if !hasExplicitUsername && hasExplicitHostname {
		errorMsg = "missing 'username' (you provided 'hostname' but both are required)"
	} else {
		errorMsg = "missing identity configuration for ReplicationDestination"
	}

	// Add brief usage hint and link to documentation
	errorMsg = fmt.Sprintf("Kopia ReplicationDestination error: %s\n\n"+
		"Provide either:\n"+
		"  1. Both 'username' and 'hostname' fields, OR\n"+
		"  2. A 'sourceIdentity' section with 'sourceName'\n\n"+
		"For detailed examples, see: https://volsync.readthedocs.io/en/stable/usage/kopia/index.html",
		errorMsg)

	return fmt.Errorf("%s", errorMsg)
}
