# Set versions to be used in the build here

# VERSION defines the project version for the bundle.
# Update this value when you upgrade the version of your project.
# To re-generate a bundle for another specific version without changing the standard setup, you can:
# - use the VERSION as arg of the bundle target (e.g make bundle VERSION=0.0.2)
# - use environment variables to overwrite this value (e.g export VERSION=0.0.2)

# Bundle Version being built right now and channels to use
VERSION := 0.4.0
CHANNELS := stable,acm-2.5
DEFAULT_CHANNEL := stable
MIN_KUBE_VERSION := 1.20.0

HEAD_COMMIT ?= $(shell git rev-parse --short HEAD)
DIRTY ?= $(shell git diff --quiet || echo '-dirty')
BUILD_VERSION := v$(VERSION)+$(HEAD_COMMIT)$(DIRTY)

BUILDDATE := $(shell date -u '+%Y-%m-%dT%H:%M:%S.%NZ')
