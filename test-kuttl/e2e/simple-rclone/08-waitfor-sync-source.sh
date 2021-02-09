#! /bin/bash

set -e -o pipefail

while [[ $(kubectl -n "$NAMESPACE" get ReplicationSource/source -o jsonpath="{.status.conditions[0].lastTransitionTime}}") == "<no value>" ]]; do
    sleep 1
done

echo "ReplicationSource is synced"