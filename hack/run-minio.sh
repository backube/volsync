#! /bin/bash

set -e -o pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

# Makes minio available at minio.${MINIO_NAMESPACE}.svc.cluster.local:9000

# Namespace to deploy MinIO into
MINIO_NAMESPACE="${MINIO_NAMESPACE:-minio}"
# Non-zero indicates MinIO should be deployed w/ a self-signed cert & https
MINIO_USE_TLS=${MINIO_USE_TLS:-0}
# Specific MinIO chart version to use
MINIO_CHART_VERSION="${MINIO_CHART_VERSION:-5.4.0}"

# A caller can provide a pre-built image as the first argument or via
# MINIO_IMAGE. Without one, build and load the local test image into kind.
MINIO_IMAGE="${1:-${MINIO_IMAGE:-}}"
if [[ -z "${MINIO_IMAGE}" ]]; then
    MINIO_IMAGE="${MINIO_TEST_IMAGE:-volsync-test-storage:local}"
    MINIO_TEST_ARCH="${MINIO_TEST_ARCH:-$(uname -m | sed -e 's/x86_64/amd64/' -e 's/aarch64/arm64/')}"
    REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"

    make --no-print-directory -C "${REPO_ROOT}" minio-test-storage-build \
        MINIO_TEST_ARCH="${MINIO_TEST_ARCH}" \
        MINIO_TEST_IMAGE="${MINIO_IMAGE}"

    if ! command -v kind >/dev/null 2>&1; then
        echo "kind is required to load the locally built MinIO image" >&2
        exit 1
    fi
    kind load docker-image "${MINIO_IMAGE}"
fi

MC_IMAGE="${MC_IMAGE:-${MINIO_IMAGE}}"

split_image_ref() {
    local image_ref="$1"
    local image_name="${image_ref##*/}"

    if [[ "${image_name}" != *:* ]]; then
        echo "Image must include a tag: ${image_ref}" >&2
        return 1
    fi

    IMAGE_REPOSITORY="${image_ref%:*}"
    IMAGE_TAG="${image_ref##*:}"
}

split_image_ref "${MINIO_IMAGE}"
MINIO_IMAGE_REPOSITORY="${IMAGE_REPOSITORY}"
MINIO_IMAGE_TAG="${IMAGE_TAG}"
split_image_ref "${MC_IMAGE}"
MC_IMAGE_REPOSITORY="${IMAGE_REPOSITORY}"
MC_IMAGE_TAG="${IMAGE_TAG}"

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
    --set-string "image.repository=${MINIO_IMAGE_REPOSITORY}" \
    --set-string "image.tag=${MINIO_IMAGE_TAG}" \
    --set-string "mcImage.repository=${MC_IMAGE_REPOSITORY}" \
    --set-string "mcImage.tag=${MC_IMAGE_TAG}" \
    --set image.pullPolicy=IfNotPresent \
    --set mcImage.pullPolicy=IfNotPresent \
    --version "${MINIO_CHART_VERSION}" \
    --wait --timeout=300s \
    minio minio/minio; then
    kubectl -n "${MINIO_NAMESPACE}" describe all,pvc,pv
    exit 1
fi
