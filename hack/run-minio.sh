#! /bin/bash

set -e -o pipefail

MINIO_NAMESPACE="minio"

# Delete minio if it's already there
kubectl delete ns "${MINIO_NAMESPACE}" || true

helm repo add bitnami https://charts.bitnami.com/bitnami
helm repo update

# Makes minio available at minio.minio.svc.cluster.local:9000
# The chart version needs to be in sync with the version used here:
# https://github.com/openshift/release/blob/master/ci-operator/config/backube/volsync/backube-volsync-main.yaml

# Detect OpenShift
declare -a SECURITY_ARGS
if kubectl api-resources --api-group security.openshift.io | grep -qi SecurityContextConstraints; then
    echo "===> Detected OpenShift <==="
    SECURITY_ARGS=(--set "containerSecurityContext.enabled=false" --set "podSecurityContext.enabled=false")
else
    echo "===> Not running on OpenShift <==="
    SECURITY_ARGS=(--set "securityContext.enabled=false" --set "volumePermissions.enabled=true")
fi

if ! helm install --create-namespace -n "${MINIO_NAMESPACE}" \
    --debug \
    --set auth.rootUser=access \
    --set auth.rootPassword=password \
    --set defaultBuckets=mybucket \
    "${SECURITY_ARGS[@]}" \
    --version 11.6.3 \
    --wait --timeout=300s \
    minio bitnami/minio; then
    kubectl -n "${MINIO_NAMESPACE}" describe all,pvc,pv
    exit 1
fi
