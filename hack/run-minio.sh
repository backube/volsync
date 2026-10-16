#!/usr/bin/env bash

set -euo pipefail

# Temporary compatibility for existing Prow commands. This only provisions RustFS.
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

case "${MINIO_NAMESPACE:-minio}" in
    minio) default_rustfs_namespace="rustfs" ;;
    minio-tls) default_rustfs_namespace="rustfs-tls" ;;
    *)
        echo "The temporary Prow wrapper supports MINIO_NAMESPACE=minio or minio-tls." >&2
        exit 2
        ;;
esac

export RUSTFS_NAMESPACE="${RUSTFS_NAMESPACE:-${default_rustfs_namespace}}"
export RUSTFS_USE_TLS="${RUSTFS_USE_TLS:-${MINIO_USE_TLS:-0}}"

exec "${SCRIPT_DIR}/run-rustfs.sh" "$@"
