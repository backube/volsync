IMAGE := quay.io/backube/scribe-mover-rsync

BUILDDATE := $(shell date -u '+%Y-%m-%dT%H:%M:%S.%NZ')
VERSION := $(shell git describe --match 'v[0-9]*' --tags --dirty 2> /dev/null || git describe --always --dirty)

.PHONY: all
all: image

.PHONY: build
image:
	docker build \
	  --build-arg "builddate=$(BUILDDATE)" \
	  --build-arg "version=$(VERSION)" \
	  -t $(IMAGE) \
	  -f Dockerfile .
