#! /bin/bash

set -e -o pipefail

SNAPNAME=$(kubectl -n "$NAMESPACE" get ReplicationDestination/test -otemplate="{{.status.latestImage.name}}")
kubectl -n "$NAMESPACE" apply -f - <<EOF
---
kind: PersistentVolumeClaim
apiVersion: v1
metadata:
  name: data-dest
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
  dataSource:
    apiGroup: snapshot.storage.k8s.io
    kind: VolumeSnapshot
    name: $SNAPNAME
EOF
