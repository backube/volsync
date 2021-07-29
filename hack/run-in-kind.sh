#! /bin/bash

set -e -o pipefail

# cd to top dir
scriptdir="$(dirname "$(realpath "$0")")"
cd "$scriptdir/.."

# Build the container images
make docker-build
make -C mover-rclone image
make -C mover-restic image
make -C mover-rsync image

# Load the images into kind
# We are using a special tag that should never be pushed to a repo so that it's
# obvious if we try to run a container other than these intended ones.
KIND_TAG=local-build
IMAGES=(
        "quay.io/backube/volsync"
        "quay.io/backube/volsync-mover-rclone"
        "quay.io/backube/volsync-mover-restic"
        "quay.io/backube/volsync-mover-rsync"
)
for i in "${IMAGES[@]}"; do
    docker tag "${i}:latest" "${i}:${KIND_TAG}"
    kind load docker-image "${i}:${KIND_TAG}"
done

helm upgrade --install --create-namespace -n volsync-system \
    --set image.tag="${KIND_TAG}" \
    --set rclone.tag="${KIND_TAG}" \
    --set restic.tag="${KIND_TAG}" \
    --set rsync.tag="${KIND_TAG}" \
    --set metrics.disableAuth=true \
    volsync ./helm/volsync
