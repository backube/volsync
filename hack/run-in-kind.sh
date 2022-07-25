#! /bin/bash

set -e -o pipefail

# check if commands exist
for cmd in helm docker kind; do
	if ! command -v $cmd >/dev/null 2>&1; then
		echo "Error: $cmd is not installed"
		exit 1
	fi
done

# cd to top dir
scriptdir="$(dirname "$(realpath "$0")")"
cd "$scriptdir/.."

# Build the container images
make docker-build cli
make -C mover-rclone image
make -C mover-restic image
make -C mover-rsync image
make -C mover-syncthing image

# Load the images into kind
# We are using a special tag that should never be pushed to a repo so that it's
# obvious if we try to run a container other than these intended ones.
KIND_TAG=local-build
IMAGES=(
        "quay.io/backube/volsync"
        "quay.io/backube/volsync-mover-rclone"
        "quay.io/backube/volsync-mover-restic"
        "quay.io/backube/volsync-mover-rsync"
        "quay.io/backube/volsync-mover-syncthing"
)
for i in "${IMAGES[@]}"; do
    docker tag "${i}:latest" "${i}:${KIND_TAG}"
    kind load docker-image "${i}:${KIND_TAG}"
done

# Pre-cache the busybox image
docker pull busybox
kind load docker-image busybox

helm upgrade --install --create-namespace -n volsync-system \
    --debug \
    --set image.tag="${KIND_TAG}" \
    --set rclone.tag="${KIND_TAG}" \
    --set restic.tag="${KIND_TAG}" \
    --set rsync.tag="${KIND_TAG}" \
    --set syncthing.tag="${KIND_TAG}" \
    --set metrics.disableAuth=true \
    --wait --timeout=300s \
    volsync ./helm/volsync
