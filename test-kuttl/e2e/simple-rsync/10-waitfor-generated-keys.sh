#! /bin/bash

set -e -o pipefail
echo "***************************START*****************************"

echo "$NAMESPACE"
echo $(kubectl -n "$NAMESPACE" get ReplicationDestination/test -o yaml)

while [[ $(kubectl -n "$NAMESPACE" get ReplicationDestination/test -otemplate="{{.status.rsync.sshKeys}}") == "<no value>" ]]; do
    echo "********************************88"
   
    sleep 1
done
echo "***************************END*****************************"
