#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=hack/rustfs-test-assets.env
source "${SCRIPT_DIR}/rustfs-test-assets.env"

RUSTFS_NAMESPACE="${RUSTFS_NAMESPACE:-rustfs}"
RUSTFS_USE_TLS="${RUSTFS_USE_TLS-0}"
RUSTFS_STORAGE_CLASS="${RUSTFS_STORAGE_CLASS:-}"
RUSTFS_CERT_SECRET="rustfs-cert"
RUSTFS_CHART_REPOSITORY="https://charts.rustfs.com"
RUSTFS_CLI_IMAGE="rustfs/rc:${RUSTFS_CLI_VERSION}"

case "${RUSTFS_USE_TLS}" in
    0|1) ;;
    *)
        echo "RUSTFS_USE_TLS must be either 0 or 1." >&2
        exit 1
        ;;
esac

helm_error() {
    echo "$1" >&2
    echo "Run 'make helm' to download the repository-pinned Helm into bin/helm, or set HELM_BIN to another Helm 3 or 4 executable." >&2
    exit 1
}

if [[ -z "${HELM_BIN:-}" ]]; then
    if [[ -x "${SCRIPT_DIR}/../bin/helm" ]]; then
        HELM_BIN="${SCRIPT_DIR}/../bin/helm"
    elif command -v helm >/dev/null 2>&1; then
        HELM_BIN="$(command -v helm)"
    else
        helm_error "Helm was not found."
    fi
fi
if [[ "${HELM_BIN}" == */* && ! -x "${HELM_BIN}" ]]; then
    helm_error "Helm executable not found: ${HELM_BIN}"
fi
if [[ "${HELM_BIN}" != */* ]] && ! command -v "${HELM_BIN}" >/dev/null 2>&1; then
    helm_error "Helm executable not found: ${HELM_BIN}"
fi
if ! helm_version="$("${HELM_BIN}" version --template '{{.Version}}' 2>/dev/null)"; then
    helm_error "Could not read the Helm version from '${HELM_BIN}'."
fi
if [[ "${helm_version}" =~ ^v?([0-9]+)\. ]]; then
    helm_major="${BASH_REMATCH[1]}"
else
    helm_error "Could not determine the Helm major version from '${helm_version}'."
fi
case "${helm_major}" in
    3|4) ;;
    *) helm_error "Helm ${helm_version} is not supported; this script requires Helm major version 3 or 4." ;;
esac

if [[ ${#RUSTFS_NAMESPACE} -gt 63 || ! "${RUSTFS_NAMESPACE}" =~ ^[a-z0-9]([-a-z0-9]*[a-z0-9])?$ ]]; then
    echo "RUSTFS_NAMESPACE must be a valid Kubernetes namespace name" >&2
    exit 1
fi

query_default_storage_classes() {
    local annotation="$1"
    local template='{{range .items}}{{if .metadata.annotations}}'
    template+="{{if eq (printf \"%v\" (index .metadata.annotations \"${annotation}\")) \"true\"}}"
    template+='{{.metadata.name}}{{"\n"}}{{end}}{{end}}{{end}}'
    kubectl get storageclasses -o "go-template=${template}"
}

if [[ -z "${RUSTFS_STORAGE_CLASS}" ]]; then
    # printf stringifies missing annotation values for the template comparison.
    # Prefer GA defaults; query the beta annotation only when no GA default exists.
    if ! default_storage_classes="$(query_default_storage_classes "storageclass.kubernetes.io/is-default-class")"; then
        echo "Could not query StorageClasses; set RUSTFS_STORAGE_CLASS explicitly and verify kubectl access." >&2
        exit 1
    fi
    if [[ -z "${default_storage_classes}" ]]; then
        if ! default_storage_classes="$(query_default_storage_classes "storageclass.beta.kubernetes.io/is-default-class")"; then
            echo "Could not query StorageClasses; set RUSTFS_STORAGE_CLASS explicitly and verify kubectl access." >&2
            exit 1
        fi
    fi
    if [[ -z "${default_storage_classes}" ]]; then
        echo "No default StorageClass is configured; set RUSTFS_STORAGE_CLASS to a class name." >&2
        exit 1
    fi
    if [[ "${default_storage_classes}" == *$'\n'* ]]; then
        echo "Multiple default StorageClasses are configured; set RUSTFS_STORAGE_CLASS explicitly." >&2
        exit 1
    fi
    RUSTFS_STORAGE_CLASS="${default_storage_classes}"
fi
if [[ ! "${RUSTFS_STORAGE_CLASS}" =~ ^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$ ]]; then
    echo "RUSTFS_STORAGE_CLASS must be a valid Kubernetes storage class name" >&2
    exit 1
fi

if kubectl api-resources --api-group security.openshift.io 2>/dev/null | grep -qi SecurityContextConstraints; then
    IS_OPENSHIFT=1
    echo "===> Detected OpenShift; allowing SCC-assigned IDs and groups <==="
else
    IS_OPENSHIFT=0
    echo "===> Not running on OpenShift <==="
fi

VALUES_FILE="$(mktemp)"
CERTIFICATE_DIR=""
HELM4_PLUGINS_ROOT=""
cleanup() {
    rm -f "${VALUES_FILE}"
    if [[ -n "${CERTIFICATE_DIR}" ]]; then
        rm -rf "${CERTIFICATE_DIR}"
    fi
    if [[ -n "${HELM4_PLUGINS_ROOT}" ]]; then
        rm -rf "${HELM4_PLUGINS_ROOT}"
    fi
}
trap cleanup EXIT

prepare_helm4_post_renderer() {
    HELM4_PLUGINS_ROOT="$(mktemp -d)"
    local plugin_dir="${HELM4_PLUGINS_ROOT}/rustfs-https-probes"
    mkdir "${plugin_dir}"
    cp "${SCRIPT_DIR}/rustfs-https-probes.py" "${plugin_dir}/rustfs-https-probes.py"
    chmod +x "${plugin_dir}/rustfs-https-probes.py"
    cat >"${plugin_dir}/plugin.yaml" <<'EOF'
apiVersion: v1
type: postrenderer/v1
name: rustfs-https-probes
version: 0.1.0
runtime: subprocess
runtimeConfig:
  platformCommand:
    - command: ${HELM_PLUGIN_DIR}/rustfs-https-probes.py
EOF
}

write_values() {
    local enable_tls="$1"
    local pod_identity_values
    if [[ "${IS_OPENSHIFT}" == "1" ]]; then
        # Null removes chart defaults so the SCC can assign IDs and groups.
        pod_identity_values=$'  fsGroup: null\n  runAsGroup: null\n  runAsUser: null'
    else
        pod_identity_values=$'  fsGroup: 10001\n  runAsGroup: 10001\n  runAsUser: 10001'
    fi
    cat >"${VALUES_FILE}" <<EOF
fullnameOverride: rustfs
replicaCount: 1
drivesPerNode: 1
mode:
  standalone:
    enabled: true
  distributed:
    enabled: false
ingress:
  enabled: false
image:
  rustfs:
    repository: rustfs/rustfs
    tag: "${RUSTFS_SERVER_VERSION}"
    pullPolicy: IfNotPresent
  initImage:
    tag: "${BUSYBOX_VERSION}"
secret:
  rustfs:
    access_key: access
    secret_key: password
podSecurityContext:
  runAsNonRoot: true
  seccompProfile:
    type: RuntimeDefault
${pod_identity_values}
containerSecurityContext:
  allowPrivilegeEscalation: false
  capabilities:
    drop:
      - ALL
  readOnlyRootFilesystem: true
  runAsNonRoot: true
storageclass:
  name: "${RUSTFS_STORAGE_CLASS}"
  dataStorageSize: 8Gi
  logStorageSize: 1Gi
resources:
  requests:
    cpu: 100m
    memory: 512Mi
EOF

    if [[ "${enable_tls}" == "1" ]]; then
        cat >>"${VALUES_FILE}" <<EOF
extraEnv:
  - name: RUSTFS_TLS_PATH
    value: /opt/tls/
extraVolumes:
  - name: rustfs-tls
    secret:
      secretName: ${RUSTFS_CERT_SECRET}
      items:
        - key: rustfs_cert.pem
          path: rustfs_cert.pem
        - key: rustfs_key.pem
          path: rustfs_key.pem
extraVolumeMounts:
  - name: rustfs-tls
    mountPath: /opt/tls/
    readOnly: true
EOF
    fi
}

install_rustfs() {
    local enable_tls="$1"
    write_values "${enable_tls}"
    local -a helm_args=(
        upgrade --install rustfs rustfs
        --repo "${RUSTFS_CHART_REPOSITORY}"
        --version "${RUSTFS_CHART_VERSION}"
        --namespace "${RUSTFS_NAMESPACE}"
        --create-namespace
        --values "${VALUES_FILE}"
        --wait --timeout=300s
    )
    local use_helm4_plugin=0
    if [[ "${enable_tls}" == "1" ]]; then
        if [[ "${helm_major}" == "3" ]]; then
            helm_args+=(--post-renderer "${SCRIPT_DIR}/rustfs-https-probes.py")
        else
            prepare_helm4_post_renderer
            use_helm4_plugin=1
        fi
    fi
    if [[ "${use_helm4_plugin}" == "1" ]]; then
        helm_args+=(--post-renderer rustfs-https-probes)
    fi
    local helm_status=0
    if [[ "${use_helm4_plugin}" == "1" ]]; then
        env "HELM_PLUGINS=${HELM4_PLUGINS_ROOT}" "${HELM_BIN}" "${helm_args[@]}" || helm_status=$?
    else
        "${HELM_BIN}" "${helm_args[@]}" || helm_status=$?
    fi
    if [[ "${helm_status}" -ne 0 ]]; then
        kubectl -n "${RUSTFS_NAMESPACE}" get all,pvc 2>/dev/null || true
        return 1
    fi
}

ensure_tls_certificate() {
    if kubectl -n "${RUSTFS_NAMESPACE}" get secret "${RUSTFS_CERT_SECRET}" >/dev/null 2>&1; then
        return
    fi

    CERTIFICATE_DIR="$(mktemp -d)"
    openssl req -x509 -newkey rsa:2048 -days 3650 -noenc \
        -keyout "${CERTIFICATE_DIR}/rustfs_key.pem" \
        -out "${CERTIFICATE_DIR}/rustfs_cert.pem" \
        -subj "/CN=rustfs-svc.${RUSTFS_NAMESPACE}.svc.cluster.local" \
        -addext "subjectAltName=DNS:rustfs-svc.${RUSTFS_NAMESPACE},DNS:rustfs-svc.${RUSTFS_NAMESPACE}.svc,DNS:rustfs-svc.${RUSTFS_NAMESPACE}.svc.cluster.local"
    kubectl -n "${RUSTFS_NAMESPACE}" create secret generic "${RUSTFS_CERT_SECRET}" \
        --from-file="rustfs_cert.pem=${CERTIFICATE_DIR}/rustfs_cert.pem" \
        --from-file="rustfs_key.pem=${CERTIFICATE_DIR}/rustfs_key.pem" \
        --dry-run=client -o yaml | kubectl apply -f -
    rm -rf "${CERTIFICATE_DIR}"
    CERTIFICATE_DIR=""
}

initialize_bucket() {
    local job_name
    local pod_identity_values=""
    local container_identity_values=""
    if [[ "${IS_OPENSHIFT}" != "1" ]]; then
        pod_identity_values=$'        fsGroup: 10001\n        runAsGroup: 10001\n        runAsUser: 10001'
        container_identity_values=$'            runAsUser: 10001'
    fi
    job_name="rustfs-bucket-init-$(date +%s)-$$"
    kubectl -n "${RUSTFS_NAMESPACE}" apply -f - <<EOF
---
apiVersion: batch/v1
kind: Job
metadata:
  name: ${job_name}
spec:
  activeDeadlineSeconds: 300
  backoffLimit: 2
  template:
    spec:
      automountServiceAccountToken: false
      securityContext:
        runAsNonRoot: true
        seccompProfile:
          type: RuntimeDefault
${pod_identity_values}
      containers:
        - name: bucket-init
          image: ${RUSTFS_CLI_IMAGE}
          imagePullPolicy: IfNotPresent
          command: ["/bin/sh", "-c"]
          args:
            - |
              set -eu
              # Keep both retry loops' sleep budget well below activeDeadlineSeconds.
              retry_attempts=30
              retry_interval_seconds=2
              attempt=1
              until rc alias set \\
                 rustfs \\
                 "http://rustfs-svc.\${RUSTFS_NAMESPACE}.svc.cluster.local:9000" \\
                 "\${RUSTFS_ACCESS_KEY}" \\
                 "\${RUSTFS_SECRET_KEY}" \\
                 --region us-east-1 \\
                 --bucket-lookup path >/dev/null 2>&1; do
                if [ "\${attempt}" -ge "\${retry_attempts}" ]; then
                  echo "RustFS alias could not be configured after \${retry_attempts} attempts." >&2
                  exit 1
                fi
                sleep "\${retry_interval_seconds}"
                attempt=\$((attempt + 1))
              done

              attempt=1
              until rc ls rustfs/restic-e2e >/dev/null 2>&1; do
                if rc mb rustfs/restic-e2e >/dev/null 2>&1; then
                  break
                fi
                if [ "\${attempt}" -ge "\${retry_attempts}" ]; then
                  if rc ls rustfs/restic-e2e >/dev/null 2>&1; then
                    break
                  fi
                  echo "RustFS bucket 'restic-e2e' could not be found or created after \${retry_attempts} attempts." >&2
                  exit 1
                fi
                sleep "\${retry_interval_seconds}"
                attempt=\$((attempt + 1))
              done
          env:
            - name: HOME
              value: /tmp
            - name: RUSTFS_NAMESPACE
              value: "${RUSTFS_NAMESPACE}"
            - name: RUSTFS_ACCESS_KEY
              valueFrom:
                secretKeyRef:
                  name: rustfs-secret
                  key: RUSTFS_ACCESS_KEY
            - name: RUSTFS_SECRET_KEY
              valueFrom:
                secretKeyRef:
                  name: rustfs-secret
                  key: RUSTFS_SECRET_KEY
          securityContext:
            allowPrivilegeEscalation: false
            capabilities:
              drop: [ALL]
            readOnlyRootFilesystem: true
            runAsNonRoot: true
${container_identity_values}
          volumeMounts:
            - name: tmp
              mountPath: /tmp
      restartPolicy: Never
      volumes:
        - name: tmp
          emptyDir: {}
EOF

    if ! kubectl -n "${RUSTFS_NAMESPACE}" wait --for=condition=complete \
        --timeout=300s "job/${job_name}"; then
        kubectl -n "${RUSTFS_NAMESPACE}" logs "job/${job_name}" || true
        kubectl -n "${RUSTFS_NAMESPACE}" describe "job/${job_name}" || true
        kubectl -n "${RUSTFS_NAMESPACE}" delete job "${job_name}" --ignore-not-found
        return 1
    fi
    kubectl -n "${RUSTFS_NAMESPACE}" logs "job/${job_name}"
    kubectl -n "${RUSTFS_NAMESPACE}" delete job "${job_name}" --ignore-not-found
}

# The CLI initializer uses HTTP so that it does not need to trust the local
# self-signed certificate. TLS is enabled only after the bucket exists.
install_rustfs 0
initialize_bucket

if [[ "${RUSTFS_USE_TLS}" != "0" ]]; then
    ensure_tls_certificate
    install_rustfs 1
fi
