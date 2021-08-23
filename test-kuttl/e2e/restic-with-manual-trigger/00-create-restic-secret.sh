#! /bin/bash

set -e -o pipefail

MINIO_ACCESS_KEY=$(kubectl get secret --namespace minio minio -o jsonpath="{.data.access-key}" | base64 --decode)
MINIO_SECRET_KEY=$(kubectl get secret --namespace minio minio -o jsonpath="{.data.secret-key}" | base64 --decode)

kubectl create -n "$NAMESPACE" -f - <<EOF
---
apiVersion: v1
kind: Secret
metadata:
  name: restic-repo
type: Opaque
stringData:
  RESTIC_REPOSITORY: s3:http://minio.minio.svc.cluster.local:9000/restic-manual-trigger
  RESTIC_PASSWORD: ThisIsTheResticPassword
  AWS_ACCESS_KEY_ID: ${MINIO_ACCESS_KEY}
  AWS_SECRET_ACCESS_KEY: ${MINIO_SECRET_KEY}
EOF
