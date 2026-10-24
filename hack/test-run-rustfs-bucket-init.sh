#!/usr/bin/env bash
#
# Cluster-free checks for the RustFS bucket-init container script and for the
# failure diagnostics emitted by run-rustfs.sh.

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
TEST_DIR="$(mktemp -d)"
trap 'rm -rf "${TEST_DIR}"' EXIT
TEST_BIN="${TEST_DIR}/bin"
mkdir -p "${TEST_BIN}"

fail() {
    echo "$1" >&2
    exit 1
}

cat >"${TEST_BIN}/rc" <<'EOF'
#!/usr/bin/env bash
set -eu

subcommand="${1:-}"
marker="FAKE_RC_STDERR_MARKER"

case "${subcommand}" in
    alias)
        if [[ "${RC_TEST_CASE}" == "hang" ]]; then
            echo "${marker}: alias set is hanging" >&2
            exec sleep 30
        fi
        exit 0
        ;;
    ls)
        case "${RC_TEST_CASE}" in
            success)
                exit 0
                ;;
            create-bucket)
                count=0
                if [[ -f "${RC_STATE_DIR}/ls-count" ]]; then
                    count="$(<"${RC_STATE_DIR}/ls-count")"
                fi
                count=$((count + 1))
                printf '%s\n' "${count}" >"${RC_STATE_DIR}/ls-count"
                if ((count == 1)); then
                    echo "${marker}: bucket is missing" >&2
                    exit 1
                fi
                exit 0
                ;;
            exhaustion)
                echo "${marker}: ls cannot reach the server" >&2
                exit 1
                ;;
            *)
                echo "${marker}: unexpected ls test case" >&2
                exit 1
                ;;
        esac
        ;;
    mb)
        case "${RC_TEST_CASE}" in
            create-bucket)
                exit 0
                ;;
            exhaustion)
                echo "${marker}: mb cannot reach the server" >&2
                exit 1
                ;;
            *)
                echo "${marker}: unexpected mb test case" >&2
                exit 1
                ;;
        esac
        ;;
    *)
        echo "${marker}: unexpected rc invocation" >&2
        exit 1
        ;;
esac
EOF

# --- Part A: in-container bucket-init script -------------------------------

chmod +x "${TEST_BIN}/rc"

access_key_sentinel="ACCESS_KEY_SENTINEL"
secret_key_sentinel="SECRET_KEY_SENTINEL"
bucket_init_status=0
bucket_init_output=""

run_bucket_init_case() {
    local test_case="$1"
    local state_dir="${TEST_DIR}/state-${test_case}"

    mkdir -p "${state_dir}"
    bucket_init_output="${TEST_DIR}/bucket-init-${test_case}.log"
    bucket_init_status=0

    RC_TEST_CASE="${test_case}" \
        RC_STATE_DIR="${state_dir}" \
        PATH="${TEST_BIN}:${PATH}" \
        RUSTFS_NAMESPACE=rustfs \
        RUSTFS_ACCESS_KEY="${access_key_sentinel}" \
        RUSTFS_SECRET_KEY="${secret_key_sentinel}" \
        RUSTFS_BUCKET_INIT_LOG="${TEST_DIR}/rc-${test_case}.log" \
        RUSTFS_BUCKET_INIT_RETRY_ATTEMPTS=2 \
        RUSTFS_BUCKET_INIT_RETRY_INTERVAL_SECONDS=0 \
        RUSTFS_BUCKET_INIT_DEADLINE_SECONDS=20 \
        RUSTFS_BUCKET_INIT_COMMAND_TIMEOUT_SECONDS=1 \
        timeout 20 "${SCRIPT_DIR}/rustfs-bucket-init.sh" \
        >"${bucket_init_output}" 2>&1 || bucket_init_status=$?
}

# The outer timeout uses status 124 when the script itself hangs.
assert_bucket_init_did_not_hang() {
    if [[ "${bucket_init_status}" -eq 124 ]]; then
        fail "The bucket-init script did not finish within the 20s test timeout."
    fi
}

run_bucket_init_case success
[[ "${bucket_init_status}" -eq 0 ]] || fail "Successful bucket init returned ${bucket_init_status}."
grep -Fq 'rc alias set succeeded' "${bucket_init_output}" || fail "Missing alias success progress line."
grep -Fq "bucket 'restic-e2e' is ready" "${bucket_init_output}" || fail "Missing bucket ready message."
if grep -Fq "${access_key_sentinel}" "${bucket_init_output}" ||
    grep -Fq "${secret_key_sentinel}" "${bucket_init_output}"; then
    fail "Bucket-init output leaked access or secret key values."
fi

run_bucket_init_case hang
assert_bucket_init_did_not_hang
[[ "${bucket_init_status}" -ne 0 ]] || fail "A hanging rc call did not fail the bucket init."
grep -Fq 'timed out or was terminated after 1s' "${bucket_init_output}" || fail "Missing per-command timeout progress line."
grep -Fq 'FAKE_RC_STDERR_MARKER' "${bucket_init_output}" || fail "Captured rc stderr was not printed on exhaustion."
grep -Fq 'captured rc output' "${bucket_init_output}" || fail "Missing rc output dump header."
grep -Fq 'RustFS alias could not be configured' "${bucket_init_output}" || fail "Missing high-level alias failure."

run_bucket_init_case create-bucket
[[ "${bucket_init_status}" -eq 0 ]] || fail "Bucket creation path returned ${bucket_init_status}."
grep -Fq 'rc ls rustfs/restic-e2e failed (status 1)' "${bucket_init_output}" || fail "Missing first ls failure line."
grep -Fq 'rc mb rustfs/restic-e2e succeeded' "${bucket_init_output}" || fail "Missing mb success line."
grep -Fq "bucket 'restic-e2e' is ready" "${bucket_init_output}" || fail "Bucket creation did not report readiness."
if grep -Fq 'FAKE_RC_STDERR_MARKER' "${bucket_init_output}"; then
    fail "Captured rc output should only be dumped when the retries are exhausted."
fi

run_bucket_init_case exhaustion
assert_bucket_init_did_not_hang
[[ "${bucket_init_status}" -ne 0 ]] || fail "Exhausted bucket retries did not fail."
grep -Fq 'rc ls rustfs/restic-e2e failed (status 1)' "${bucket_init_output}" || fail "Missing ls failure line."
grep -Fq 'rc mb rustfs/restic-e2e failed (status 1)' "${bucket_init_output}" || fail "Missing mb failure line."
grep -Fq 'FAKE_RC_STDERR_MARKER: mb cannot reach the server' "${bucket_init_output}" ||
    fail "Captured rc stderr was not printed on exhaustion."
grep -Fq 'captured rc output' "${bucket_init_output}" || fail "Missing rc output dump header."

# A zero per-command timeout would disable timeout(1)'s bound entirely.
invalid_output="${TEST_DIR}/bucket-init-invalid.log"
invalid_status=0
PATH="${TEST_BIN}:${PATH}" \
    RUSTFS_NAMESPACE=rustfs \
    RUSTFS_ACCESS_KEY="${access_key_sentinel}" \
    RUSTFS_SECRET_KEY="${secret_key_sentinel}" \
    RUSTFS_BUCKET_INIT_COMMAND_TIMEOUT_SECONDS=0 \
    "${SCRIPT_DIR}/rustfs-bucket-init.sh" >"${invalid_output}" 2>&1 || invalid_status=$?
[[ "${invalid_status}" -ne 0 ]] || fail "A zero command timeout should be rejected."
grep -Fq 'must be greater than 0' "${invalid_output}" || fail "Missing zero-timeout validation message."

# --- Part B: host-side failure diagnostics ---------------------------------

cat >"${TEST_BIN}/kubectl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

printf '%s\n' "$*" >>"${KUBECTL_LOG}"

args=("$@")
if [[ "${args[0]:-}" == "-n" ]]; then
    args=("${args[@]:2}")
fi
command_name="${args[0]:-}"
resource="${args[1]:-}"

case "${command_name}" in
    api-resources)
        exit 0
        ;;
    apply)
        cat >"${APPLY_CAPTURE}"
        ;;
    get)
        case "${resource}" in
            storageclasses)
                printf 'standard\n'
                ;;
            job)
                printf '%s\n' "${JOB_CONDITIONS:-Failed=True}"
                ;;
            events)
                echo "EVENTS_MARKER"
                ;;
            pods)
                case " $* " in
                    *" -o name "*)
                        echo "pod/rustfs-bucket-init-fake"
                        ;;
                    *jsonpath*)
                        echo "ImagePullBackOff"
                        ;;
                    *custom-columns*)
                        echo "NAME PHASE WAITING_REASON TERMINATED_REASON RESTARTS"
                        echo "rustfs-bucket-init-fake Pending ImagePullBackOff <none> 0"
                        ;;
                    *)
                        echo "PODS_WIDE_MARKER"
                        ;;
                esac
                ;;
            *)
                echo "GET_MARKER"
                ;;
        esac
        ;;
    describe)
        if [[ "${resource}" == "job" ]]; then
            echo "DESCRIBE_JOB_MARKER"
        else
            echo "DESCRIBE_PODS_MARKER"
        fi
        ;;
    logs)
        echo "LOGS_MARKER"
        ;;
    delete)
        exit 0
        ;;
    *)
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
# Let initialize_bucket run; fake kubectl reports the Job result.
exit 0
EOF

chmod +x "${TEST_BIN}/kubectl" "${TEST_BIN}/helm"

run_rustfs_case() {
    local case_name="$1"
    local job_conditions="$2"
    local expected_status="$3"
    local wait_timeout_seconds="${4:-480}"

    kubectl_log="${TEST_DIR}/run-rustfs-${case_name}.kubectl.log"
    apply_capture="${TEST_DIR}/run-rustfs-${case_name}.job.yaml"
    run_output="${TEST_DIR}/run-rustfs-${case_name}.log"
    run_status=0

    JOB_CONDITIONS="${job_conditions}" \
        BUCKET_INIT_WAIT_TIMEOUT_SECONDS="${wait_timeout_seconds}" \
        HELM_BIN="${TEST_BIN}/helm" \
        KUBECTL_LOG="${kubectl_log}" \
        APPLY_CAPTURE="${apply_capture}" \
        PATH="${TEST_BIN}:${PATH}" \
        RUSTFS_NAMESPACE=rustfs \
        RUSTFS_USE_TLS=0 \
        RUSTFS_STORAGE_CLASS="" \
        timeout 60 "${SCRIPT_DIR}/run-rustfs.sh" >"${run_output}" 2>&1 || run_status=$?

    [[ "${run_status}" -ne 124 ]] || fail "run-rustfs.sh did not finish within the 60s test timeout for ${case_name}."
    if [[ "${expected_status}" == "success" ]]; then
        [[ "${run_status}" -eq 0 ]] || fail "run-rustfs.sh should succeed for ${case_name} (status ${run_status})."
    else
        [[ "${run_status}" -ne 0 ]] || fail "run-rustfs.sh should fail for ${case_name}."
    fi
}

# Failure case: Job fails, diagnostics must be collected, then the Job is
# deleted.
run_rustfs_case failure Failed=True failure

grep -Fq 'activeDeadlineSeconds: 420' "${apply_capture}" || fail "Job manifest is missing activeDeadlineSeconds: 420."
grep -Fq 'backoffLimit: 0' "${apply_capture}" || fail "Job manifest must not retry the whole Job."
# These patterns match the literal inlined script text, so the variables must
# not expand.
# shellcheck disable=SC2016
timeout_wrapper_pattern='              timeout -k 5 "${command_timeout_seconds}" rc'
# shellcheck disable=SC2016
rc_log_pattern='>>"${rc_output_log}" 2>&1'
grep -Fq "${timeout_wrapper_pattern}" "${apply_capture}" ||
    fail "Inlined script does not bound rc calls with timeout."
grep -Fq "${rc_log_pattern}" "${apply_capture}" || fail "Inlined script does not capture rc output."
grep -Fq '              fail_bucket_init "RustFS alias could not be configured' "${apply_capture}" ||
    fail "Bucket-init script content is not inlined with 14-space indentation."
grep -Fq 'RUSTFS_BUCKET_INIT_DEADLINE_SECONDS' "${apply_capture}" || fail "Inlined script is missing its deadline tunable."
if grep -Fq '>/dev/null 2>&1' "${apply_capture}"; then
    fail "Inlined rc calls must not discard their output."
fi

grep -Fq 'get job rustfs-bucket-init-' "${kubectl_log}" || fail "Host did not poll the Job conditions."
grep -Fq 'go-template=' "${kubectl_log}" || fail "Host poll does not read the Job conditions."
grep -Fq 'get pods -o wide' "${kubectl_log}" || fail "Diagnostics are missing 'get pods -o wide'."
grep -Fq 'custom-columns=' "${kubectl_log}" || fail "Diagnostics are missing the container status view."
grep -Fq 'describe job' "${kubectl_log}" || fail "Diagnostics are missing 'describe job'."
grep -Fq 'describe pods' "${kubectl_log}" || fail "Diagnostics are missing 'describe pods'."
grep -Fq 'logs rustfs-bucket-init-fake --all-containers --prefix' "${kubectl_log}" ||
    fail "Diagnostics are missing current pod logs."
grep -Fq 'logs rustfs-bucket-init-fake --all-containers --prefix --previous' "${kubectl_log}" ||
    fail "Diagnostics are missing previous pod logs."
grep -Fq 'logs -l app.kubernetes.io/name=rustfs --all-containers --prefix --tail=200' "${kubectl_log}" ||
    fail "Diagnostics are missing RustFS server logs."
grep -Fq 'get events --sort-by=.lastTimestamp' "${kubectl_log}" || fail "Diagnostics are missing namespace events."
grep -Fq 'The RustFS CLI image' "${run_output}" || fail "Image pull failure was not reported."

delete_lines="$(grep -n 'delete job' "${kubectl_log}" | cut -d: -f1 || true)"
[[ -n "${delete_lines}" ]] || fail "The bucket-init Job was never deleted."
[[ "$(grep -c 'delete job' "${kubectl_log}")" -eq 1 ]] || fail "The bucket-init Job should be deleted exactly once."
last_diagnostic_line="$(grep -n 'get events --sort-by=.lastTimestamp' "${kubectl_log}" | cut -d: -f1)"
if [[ "${delete_lines}" -le "${last_diagnostic_line}" ]]; then
    fail "The bucket-init Job must be deleted after diagnostics are collected."
fi

# Success case: no diagnostics, logs are printed, and the Job is deleted.
run_rustfs_case success Complete=True success

grep -Fq 'LOGS_MARKER' "${run_output}" || fail "Successful run did not print the bucket-init pod logs."
[[ "$(grep -c 'delete job' "${kubectl_log}")" -eq 1 ]] || fail "A successful run should delete the Job exactly once."
if grep -Fq 'describe job' "${kubectl_log}" || grep -Fq 'get events' "${kubectl_log}"; then
    fail "A successful run must not collect failure diagnostics."
fi

# A Job whose conditions never become terminal must still fail within the
# wait budget instead of hanging forever.
run_rustfs_case wait-expiry Pending failure 1
grep -Fq 'collecting diagnostics' "${run_output}" || fail "Wait expiry did not collect diagnostics."

echo "RustFS bucket-init and failure diagnostics checks passed."
