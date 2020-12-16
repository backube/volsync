#! /bin/bash

set -e -o pipefail

while [[ $(kubectl -n "$NAMESPACE" get ReplicationDestination/test -otemplate="{{.status.rsync.sshKeys}}") == "<no value>" ]]; do
    sleep 1
done
