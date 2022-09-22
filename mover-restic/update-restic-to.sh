#! /bin/bash

set -e -o pipefail

# Upstream repositories to fetch from
RESTIC_REPOSITORY="https://github.com/restic/restic.git"
MINIO_GO_REPOSITORY="https://github.com/minio/minio-go.git"

scriptdir="$(dirname "$(realpath "$0")")"
cd "${scriptdir}"

if [[ $# -ne 1 ]]; then
    cat - <<USAGE
Usage: $(basename "$0") <upstream-tag>
  (e.g., $(basename "$0") v0.13.1)

  For a list of available tags, try:
  git ls-remote --tags $RESTIC_REPOSITORY
USAGE
    exit 1
fi

function log() {
    echo "=====  $*  ====="
}

function update_repo_to() {
    repo="$1"
    tag="$2"
    dir="$3"
    rm -rf "$dir"
    git clone --single-branch --depth 1 -b "$tag" "$repo" "$dir"
    pushd "$dir"
    HASH="$(git rev-list -n 1 "$tag")"
    popd
    rm -rf "$dir/.git"
    echo "$repo $tag $HASH" >> SOURCE_VERSIONS
}

rm -f SOURCE_VERSIONS

############################################################
## Clone sources
############################################################

## Clone restic
RESTIC_TAG="$1"
log "Updating restic source to tag: $RESTIC_TAG"
update_repo_to "$RESTIC_REPOSITORY" "$RESTIC_TAG" restic

## Determine proper version of minio-go
MINIO_GO_TAG="$(grep github.com/minio/minio-go restic/go.mod | awk '{ print $2 }')"
log "minio-go version: $MINIO_GO_TAG"

## Clone minio-go
log "Updating minio-go source to tag: $MINIO_GO_TAG"
update_repo_to "$MINIO_GO_REPOSITORY" "$MINIO_GO_TAG" minio-go



############################################################
## Patch minio-go
############################################################

cd minio-go
# Remove sha256-simd library
find . -name '*.go' -exec sed -ri 's|github.com/minio/sha256-simd|crypto/sha256|' {} \;
# Clean up modules and verify
go mod tidy
if grep sha256-simd go.sum; then
    echo "FAILURE: Optimized sha256-simd is still present in minio-go"
    exit 1
fi
cd ..



############################################################
## Patch restic
############################################################

cd restic
# Remove sha256-simd library
find . -name '*.go' -exec sed -ri 's|github.com/minio/sha256-simd|crypto/sha256|' {} \;
# Override restic's imports of minio-go to use our patched sources
go mod edit --replace github.com/minio/minio-go/v7=../minio-go
go mod tidy
# Ensure we found everything
if grep sha256-simd go.sum; then
    echo "FAILURE: Optimized sha256-simd is still present in restic"
    exit 1
fi
cd ..
