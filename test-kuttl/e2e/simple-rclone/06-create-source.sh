#! /bin/bash

set -e -o pipefail

kubectl -n "$NAMESPACE" apply -f - <<EOF
---
apiVersion: volsync.backube/v1alpha1
kind: ReplicationSource
metadata:
  name: source
spec:
  sourcePVC: data-source
  trigger:
    schedule: "*/2 * * * *"
  rclone:
    rcloneConfigSection: "rclone-data-mover"
    rcloneDestPath: "volsync-test-bucket"
    rcloneConfig: "rclone-secret"
    copyMethod: Snapshot
EOF


