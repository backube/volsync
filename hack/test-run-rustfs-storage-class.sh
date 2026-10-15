#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
TEST_DIR="$(mktemp -d)"
trap 'rm -rf "${TEST_DIR}"' EXIT
TEST_BIN="${TEST_DIR}/bin"
mkdir -p "${TEST_BIN}"

cat >"${TEST_BIN}/kubectl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

printf '%s\n' "$*" >>"${KUBECTL_LOG}"

if [[ "${1:-}" == "api-resources" ]]; then
    exit 0
fi
if [[ "${1:-}" != "get" || "${2:-}" != "storageclasses" ]]; then
    exit 0
fi

template=""
for argument in "$@"; do
    if [[ "${argument}" == go-template=* ]]; then
        template="${argument#go-template=}"
    fi
done
if [[ "${template}" != *'printf "%v"'* ]]; then
    echo "StorageClass query does not format missing annotations safely." >&2
    exit 1
fi

case "${STORAGE_CLASS_TEST_CASE}" in
    ga)
        if [[ "${template}" != *"storageclass.kubernetes.io/is-default-class"* ]]; then
            echo "Expected the GA default annotation query." >&2
            exit 1
        fi
        if [[ "${template}" == *"storageclass.beta.kubernetes.io/is-default-class"* ]]; then
            printf 'ga-default\nbeta-stale\n'
        else
            printf 'ga-default\n'
        fi
        ;;
    beta)
        if [[ "${template}" == *"storageclass.beta.kubernetes.io/is-default-class"* ]]; then
            printf 'beta-default\n'
        fi
        ;;
    override)
        echo "StorageClass lookup should be skipped for an explicit override." >&2
        exit 1
        ;;
    *)
        echo "Unknown StorageClass test case." >&2
        exit 1
        ;;
esac
EOF

cat >"${TEST_BIN}/helm" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

if [[ "${1:-}" == "version" ]]; then
    printf 'v3.15.0\n'
    exit 0
fi
if [[ "${1:-}" == "upgrade" ]]; then
    while (($#)); do
        if [[ "${1}" == "--values" ]]; then
            cp "${2}" "${VALUES_CAPTURE}"
            break
        fi
        shift
    done
    # Stop before any install work; this test only needs the generated values.
    exit 1
fi
exit 0
EOF

chmod +x "${TEST_BIN}/kubectl" "${TEST_BIN}/helm"

run_case() {
    local test_case="$1"
    local expected_storage_class="$2"
    local override_storage_class="$3"
    local kubectl_log="${TEST_DIR}/${test_case}.kubectl.log"
    local values_capture="${TEST_DIR}/${test_case}.values.yaml"

    if STORAGE_CLASS_TEST_CASE="${test_case}" \
        KUBECTL_LOG="${kubectl_log}" \
        VALUES_CAPTURE="${values_capture}" \
        HELM_BIN="${TEST_BIN}/helm" \
        RUSTFS_NAMESPACE="rustfs" \
        RUSTFS_STORAGE_CLASS="${override_storage_class}" \
        RUSTFS_USE_TLS=0 \
        PATH="${TEST_BIN}:${PATH}" \
        "${SCRIPT_DIR}/run-rustfs.sh" >/dev/null 2>&1; then
        echo "Expected the fake Helm upgrade to stop the ${test_case} test case." >&2
        return 1
    fi

    if ! grep -Fq "  name: \"${expected_storage_class}\"" "${values_capture}"; then
        echo "${test_case} selected the wrong StorageClass." >&2
        return 1
    fi
}

run_case ga ga-default ""
if [[ "$(grep -c 'get storageclasses' "${TEST_DIR}/ga.kubectl.log")" -ne 1 ]] || \
    grep -Fq 'storageclass.beta.kubernetes.io/is-default-class' "${TEST_DIR}/ga.kubectl.log"; then
    echo "GA default lookup should not consult the beta annotation." >&2
    exit 1
fi

run_case beta beta-default ""
if [[ "$(grep -c 'get storageclasses' "${TEST_DIR}/beta.kubectl.log")" -ne 2 ]]; then
    echo "Beta lookup should run only after the GA lookup returns no default." >&2
    exit 1
fi
first_query="$(grep 'get storageclasses' "${TEST_DIR}/beta.kubectl.log" | sed -n '1p')"
second_query="$(grep 'get storageclasses' "${TEST_DIR}/beta.kubectl.log" | sed -n '2p')"
if [[ "${first_query}" != *'storageclass.kubernetes.io/is-default-class'* || \
    "${first_query}" == *'storageclass.beta.kubernetes.io/is-default-class'* || \
    "${second_query}" != *'storageclass.beta.kubernetes.io/is-default-class'* ]]; then
    echo "GA must be queried before the beta fallback." >&2
    exit 1
fi

run_case override user-selected user-selected
if grep -Fq 'get storageclasses' "${TEST_DIR}/override.kubectl.log"; then
    echo "An explicit StorageClass should skip default discovery." >&2
    exit 1
fi

WRAPPER_DIR="${TEST_DIR}/wrapper"
mkdir -p "${WRAPPER_DIR}"
cp "${SCRIPT_DIR}/run-minio.sh" "${WRAPPER_DIR}/run-minio.sh"
cat >"${WRAPPER_DIR}/run-rustfs.sh" <<'EOF'
#!/usr/bin/env bash
printf '%s %s\n' "${RUSTFS_NAMESPACE}" "${RUSTFS_USE_TLS}" >"${WRAPPER_CAPTURE}"
EOF
chmod +x "${WRAPPER_DIR}/run-minio.sh" "${WRAPPER_DIR}/run-rustfs.sh"

check_wrapper_mapping() {
    local minio_namespace="$1"
    local minio_tls="$2"
    local expected_namespace="$3"
    local expected_tls="$4"
    local capture="${TEST_DIR}/wrapper.out"

    MINIO_NAMESPACE="${minio_namespace}" \
        MINIO_USE_TLS="${minio_tls}" \
        RUSTFS_NAMESPACE="" \
        RUSTFS_USE_TLS="" \
        WRAPPER_CAPTURE="${capture}" \
        "${WRAPPER_DIR}/run-minio.sh"
    if [[ "$(cat "${capture}")" != "${expected_namespace} ${expected_tls}" ]]; then
        echo "Legacy MinIO settings did not map to the expected RustFS settings." >&2
        return 1
    fi
}

check_wrapper_mapping minio 0 rustfs 0
check_wrapper_mapping minio-tls 1 rustfs-tls 1

echo "RustFS StorageClass and Prow compatibility checks passed."
