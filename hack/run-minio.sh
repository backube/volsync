#! /bin/bash

set -e -o pipefail

helm repo add bitnami https://charts.bitnami.com/bitnami
helm repo update

helm install --create-namespace -n "$NAMESPACE" \
    --set accessKey.password=access \
    --set secretKey.password=password \
    --set securityContext.enabled=false \
    --set defaultBuckets=mybucket \
    --set volumePermissions.enabled=true \
    minio bitnami/minio
