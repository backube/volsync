#! /bin/bash

set -e -o pipefail

helm repo add bitnami https://charts.bitnami.com/bitnami
helm repo update

# Makes minio available at minio.minio.svc.cluster.local:9000
helm install --create-namespace -n minio \
    --set accessKey.password=access \
    --set secretKey.password=password \
    --set securityContext.enabled=false \
    --set defaultBuckets=mybucket \
    --set volumePermissions.enabled=true \
    minio bitnami/minio
