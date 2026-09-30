#!/bin/sh

set -eu

if [ "${1:-}" = "minio" ]; then
    shift
fi

exec /usr/bin/minio "$@"
