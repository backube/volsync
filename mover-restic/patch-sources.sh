#! /bin/bash

# Want to run this locally?
# Check out the restic and minio-go sources into this directory (See
# Dockerfile), then run this script

set -x -e -o pipefail
scriptdir="$(dirname "$(realpath "$0")")"



############################################################
## Patch minio-go
cd "${scriptdir}/minio-go"

# Replace all usage of the optimized sha256-simd w/ the standard crypto
# implementation
find . -name '*.go' -exec sed -ri 's|github.com/minio/sha256-simd|crypto/sha256|' {} \;

# Clean up module files
go mod tidy

# Ensure we found everything
if grep sha256-simd go.sum; then
    echo "FAILURE: Optimized sha256-simd is still present"
    exit 1
fi


############################################################
## Patch restic
cd "${scriptdir}/restic"

# Replace all usage of the optimized sha256-simd w/ the standard crypto
# implementation
find . -name '*.go' -exec sed -ri 's|github.com/minio/sha256-simd|crypto/sha256|' {} \;

# Override restic's imports of minio-go to use our patched sources
go mod edit --replace github.com/minio/minio-go/v7=../minio-go

# Clean up module files
go mod tidy

# Ensure we found everything
if grep sha256-simd go.sum; then
    echo "FAILURE: Optimized sha256-simd is still present"
    exit 1
fi
