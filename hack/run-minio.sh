#! /bin/bash

set -e -o pipefail

# Makes minio available at minio.${MINIO_NAMESPACE}.svc.cluster.local:9000

# Namespace to deploy MinIO into
MINIO_NAMESPACE="${MINIO_NAMESPACE:-minio}"
# Non-zero indicates MinIO should be deployed w/ a self-signed cert & https
MINIO_USE_TLS=${MINIO_USE_TLS:-0}
# Specific MinIO chart version to use
MINIO_CHART_VERSION="${MINIO_CHART_VERSION:-5.4.0}"

# Delete minio if it's already there
kubectl delete ns "${MINIO_NAMESPACE}" || true

# Get charts from minio directly
helm repo add minio https://charts.min.io/
helm repo update

# Detect OpenShift
declare -a SECURITY_ARGS
if kubectl api-resources --api-group security.openshift.io | grep -qi SecurityContextConstraints; then
    echo "===> Detected OpenShift <==="
    SECURITY_ARGS=(--set "containerSecurityContext.enabled=false" --set "securityContext.enabled=false")
else
    echo "===> Not running on OpenShift <==="
    SECURITY_ARGS=(--set "containerSecurityContext.readOnlyRootFilesystem=true")
    SECURITY_ARGS+=(--set "containerSecurityContext.allowPrivilegeEscalation=false")
    SECURITY_ARGS+=(--set "containerSecurityContext.capabilities.drop={ALL}")
    SECURITY_ARGS+=(--set "securityContext.runAsNonRoot=true")
    SECURITY_ARGS+=(--set "securityContext.seccompProfile.type=RuntimeDefault")

    SECURITY_ARGS+=(--set "postJob.securityContext.enabled=true")
    SECURITY_ARGS+=(--set "postJob.securityContext.runAsNonRoot=true")
    SECURITY_ARGS+=(--set "postJob.securityContext.seccompProfile.type=RuntimeDefault")

    SECURITY_ARGS+=(--set "makeUserJob.securityContext.enabled=true")
    SECURITY_ARGS+=(--set "makeUserJob.securityContext.runAsNonRoot=true")
    SECURITY_ARGS+=(--set "makeUserJob.securityContext.seccompProfile.type=RuntimeDefault")
    SECURITY_ARGS+=(--set "makeUserJob.containerSecurityContext.allowPrivilegeEscalation=false")
    SECURITY_ARGS+=(--set "makeUserJob.containerSecurityContext.capabilities.drop={ALL}")

    SECURITY_ARGS+=(--set "makeBucketJob.securityContext.enabled=true")
    SECURITY_ARGS+=(--set "makeBucketJob.securityContext.runAsNonRoot=true")
    SECURITY_ARGS+=(--set "makeBucketJob.securityContext.seccompProfile.type=RuntimeDefault")
    SECURITY_ARGS+=(--set "makeBucketJob.containerSecurityContext.allowPrivilegeEscalation=false")
    SECURITY_ARGS+=(--set "makeBucketJob.containerSecurityContext.capabilities.drop={ALL}")
fi

MINIO_TLS_SECRET_NAME="minio-crt"
declare -a MINIO_TLS_ARGS
if [[ MINIO_USE_TLS -ne 0 ]]; then
    MINIO_TLS_ARGS=(--set "tls.enabled=true" --set "tls.certSecret=${MINIO_TLS_SECRET_NAME}")

    # Pre-create ns and tls secret for minio
    kubectl create ns "${MINIO_NAMESPACE}"

    tmpdir="$(mktemp -d)"

    # Create self signed cert
    openssl req -x509 -newkey rsa:2048 -days 3650 \
      -noenc -keyout "${tmpdir}/private.key" -out "${tmpdir}/public.crt" \
      -subj "/CN=minio.${MINIO_NAMESPACE}.svc.cluster.local" \
      -addext "subjectAltName=DNS:minio.${MINIO_NAMESPACE},DNS:*.${MINIO_NAMESPACE},DNS:*.${MINIO_NAMESPACE}.svc.cluster.local"

    # Create generic secret that minio is expecting
    kubectl -n "${MINIO_NAMESPACE}" create secret generic "${MINIO_TLS_SECRET_NAME}" --from-file="${tmpdir}"/public.crt --from-file="${tmpdir}"/private.key

    rm -rf "${tmpdir}"
fi

if ! helm install --create-namespace -n "${MINIO_NAMESPACE}" \
    --debug \
    --set rootUser=access \
    --set rootPassword=password \
    "${SECURITY_ARGS[@]}" \
    "${MINIO_TLS_ARGS[@]}" \
    --set mode=standalone \
    --set resources.requests.memory=256Mi \
    --set persistence.size=8Gi \
    --set buckets[0].name=restic-e2e,buckets[0].policy=none,buckets[0].purge=false \
    --version "${MINIO_CHART_VERSION}" \
    --wait --timeout=300s \
    minio minio/minio; then
    kubectl -n "${MINIO_NAMESPACE}" describe all,pvc,pv
    exit 1
fi
