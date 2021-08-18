#! /bin/bash
# creates a new replicationdestination with the current restoreasof time 
set -e -o pipefail

CURRENT_TIME=$(date --rfc-3339=seconds)
TIME_ARRAY=("${CURRENT_TIME}")
DATE_TIME_STRING="${TIME_ARRAY[0]}T${TIME_ARRAY[1]}"
# save the timestamp so it can be recalled later
echo "${DATE_TIME_STRING}" > 22-timestamp.txt

kubectl create -n "$NAMESPACE" -f - <<EOF
---
apiVersion: volsync.backube/v1alpha1
kind: ReplicationDestination
metadata:
  name: restore
spec:
  trigger:
    manual: restore-once
  restic:
    repository: restic-repo
    destinationPVC: data-dest
    copyMethod: None
    cacheCapacity: 1Gi
    restoreAsOf: ${DATE_TIME_STRING}
EOF
