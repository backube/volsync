# Restic-based data mover

## Modifications to restic

By default, restic incorporates an optimized sha256 routine via the
`github.com/minio/sha256-simd` which is a drop-in replacement for
`crypto/sha256`. Unfortunately, this interferes with the ability to build a FIPS
compatible version of restic.

The `patch-sources.sh` script that runs as a part of the container build
modifies the restic and minio-go source files to use the standard
implementation.
