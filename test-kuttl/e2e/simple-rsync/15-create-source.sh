#! /bin/bash

set -e -o pipefail

KEYNAME=$(kubectl -n "$NAMESPACE" get ReplicationDestination/test -otemplate="{{.status.rsync.sshKeys}}")
ADDRESS=$(kubectl -n "$NAMESPACE" get ReplicationDestination/test -otemplate="{{.status.rsync.address}}")

kubectl -n "$NAMESPACE" apply -f - <<EOF
---
apiVersion: scribe.backube/v1alpha1
kind: ReplicationSource
metadata:
  name: source
spec:
  sourcePVC: data-source
  trigger:
    schedule: "*/2 * * * *"
  rsync:
    sshKeys: $KEYNAME
    address: $ADDRESS
    copyMethod: Clone
EOF
