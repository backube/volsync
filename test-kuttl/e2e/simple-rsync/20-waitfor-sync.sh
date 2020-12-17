#! /bin/bash

set -e -o pipefail

while [[ $(kubectl -n "$NAMESPACE" get ReplicationSource/source -otemplate="{{.status.lastSyncTime}}") == "<no value>" ]]; do
    sleep 1
done
