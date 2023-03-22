# Set versions to be used in the build here

# VERSION defines the project version for the bundle.
# Update this value when you upgrade the version of your project.
# To re-generate a bundle for another specific version without changing the standard setup, you can:
# - use the VERSION as arg of the bundle target (e.g make bundle VERSION=0.0.2)
# - use environment variables to overwrite this value (e.g export VERSION=0.0.2)

#
# Bundle Version being built right now and channels to use
#
VERSION := 0.7.1
# REPLACES_VERSION should be left empty for the first version in a new channel (See more info in Procedures.md)
REPLACES_VERSION := 0.7.0
OLM_SKIPRANGE := '>=0.4.0 <$(VERSION)'
CHANNELS := stable,stable-0.7
DEFAULT_CHANNEL := stable
MIN_KUBE_VERSION := 1.20.0

HEAD_COMMIT ?= $(shell git rev-parse --short HEAD)
DIRTY ?= $(shell git diff --quiet || echo '-dirty')
BUILD_VERSION := v$(VERSION)+$(HEAD_COMMIT)$(DIRTY)

BUILDDATE := $(shell date -u '+%Y-%m-%dT%H:%M:%S.%NZ')

# In the csv, the spec.replaces needs to be the full csv name of replacement (e.g. volsync.vX.Y.Z)
# Or, if no REPLACES_VERSION, CSV_REPLACES_VERSION should not be set
ifneq ($(strip $(REPLACES_VERSION)),)
CSV_REPLACES_VERSION := volsync.v$(REPLACES_VERSION)
endif
