#! /bin/bash

set -e -o pipefail

# Makes minio available at minio.${MINIO_NAMESPACE}.svc.cluster.local:9000

# Namespace to deploy MinIO into
MINIO_NAMESPACE="${MINIO_NAMESPACE:-minio}"
# Non-zero indicates MinIO should be deployed w/ a self-signed cert & https
MINIO_USE_TLS=${MINIO_USE_TLS:-0}
# Specific MinIO chart version to use
MINIO_CHART_VERSION="${MINIO_CHART_VERSION:-15.0.4}"

# Delete minio if it's already there
kubectl delete ns "${MINIO_NAMESPACE}" || true

helm repo add bitnami https://charts.bitnami.com/bitnami
helm repo update

# Detect OpenShift
declare -a SECURITY_ARGS
if kubectl api-resources --api-group security.openshift.io | grep -qi SecurityContextConstraints; then
    echo "===> Detected OpenShift <==="
    SECURITY_ARGS=(--set "containerSecurityContext.enabled=false" --set "podSecurityContext.enabled=false")
else
    echo "===> Not running on OpenShift <==="
fi

declare -a MINIO_TLS_ARGS
if [[ MINIO_USE_TLS -ne 0 ]]; then
    MINIO_TLS_ARGS=(--set "tls.enabled=true" --set "tls.autoGenerated=true")
fi

if ! helm install --create-namespace -n "${MINIO_NAMESPACE}" \
    --debug \
    --set auth.rootUser=access \
    --set auth.rootPassword=password \
    --set defaultBuckets=mybucket \
    "${SECURITY_ARGS[@]}" \
    "${MINIO_TLS_ARGS[@]}" \
    --version "${MINIO_CHART_VERSION}" \
    --wait --timeout=300s \
    minio bitnami/minio; then
    kubectl -n "${MINIO_NAMESPACE}" describe all,pvc,pv
    exit 1
fi
