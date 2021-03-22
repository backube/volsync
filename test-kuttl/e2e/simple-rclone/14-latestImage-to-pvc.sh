#! /bin/bash

set -e -o pipefail

SNAPNAME=$(kubectl -n "$NAMESPACE" get ReplicationDestination/destination -otemplate="{{.status.latestImage.name}}")
echo  "$SNAPNAME"
kubectl -n "$NAMESPACE" apply -f - <<EOF
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: data-dest
spec:
  accessModes:
    - ReadWriteOnce
  dataSource:
    kind: VolumeSnapshot
    apiGroup: snapshot.storage.k8s.io
    name: $SNAPNAME
  resources:
    requests:
      storage: 1Gi
EOF