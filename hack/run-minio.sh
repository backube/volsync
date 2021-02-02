#! /bin/bash

set -e -o pipefail

helm repo add bitnami https://charts.bitnami.com/bitnami
helm repo update

helm install --create-namespace -n minio \
    --set accessKey.password=access \
    --set secretKey.password=password \
    --set securityContext.enabled=false \
    --set defaultBuckets=mybucket \
    minio bitnami/minio
