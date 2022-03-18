# Common version info
include ../version.mk

IMAGE := quay.io/backube/volsync-mover-rclone

.PHONY: all
all: image

.PHONY: image
image:
	docker build \
	  --build-arg "builddate_arg=$(BUILDDATE)" \
	  --build-arg "version_arg=$(BUILD_VERSION)" \
	  -t $(IMAGE) \
	  -f Dockerfile .
