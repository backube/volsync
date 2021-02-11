#! /bin/bash

set -e -o pipefail

while [[ $(kubectl -n "$NAMESPACE" get ReplicationDestination/destination -otemplate="{{.status.latestImage.name}}") == "<no value>" ]]; do
    echo "Waiting for latest VolumeSnapShot"
    sleep 1
done

echo "ReplicationDestination is synced"