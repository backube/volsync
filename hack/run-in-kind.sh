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

# Load the images into kind
# We are using a special tag that should never be pushed to a repo so that it's
# obvious if we try to run a container other than these intended ones.
KIND_TAG=local-$(date +%s)
IMAGES=(
        "quay.io/backube/volsync"
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
    --set rsync-tls.tag="${KIND_TAG}" \
    --set syncthing.tag="${KIND_TAG}" \
    --set kopia.tag="${KIND_TAG}" \
    --set metrics.disableAuth=true \
    --wait --timeout=300s \
    volsync ./helm/volsync
