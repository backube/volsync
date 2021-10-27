#! /bin/bash

set -e -o pipefail

helm repo add bitnami https://charts.bitnami.com/bitnami
helm repo update

# Makes minio available at minio.minio.svc.cluster.local:9000
# The chart version needs to be in sync with the version used here:
# https://github.com/openshift/release/blob/master/ci-operator/config/backube/volsync/backube-volsync-main.yaml
helm install --create-namespace -n minio \
    --set accessKey.password=access \
    --set secretKey.password=password \
    --set securityContext.enabled=false \
    --set defaultBuckets=mybucket \
    --set volumePermissions.enabled=true \
    --version 9.0.5 \
    --wait --timeout=300s \
    minio bitnami/minio
