#!/bin/sh
#
# Configure the RustFS alias and the restic-e2e bucket used by the e2e tests.
#
# run-rustfs.sh inlines this script into the rustfs-bucket-init Job, where /tmp
# is a writable emptyDir and HOME=/tmp. Every rc call is bounded by timeout(1)
# and its output is captured, so a hung or failing call is reported in
# `kubectl logs` instead of silently consuming the Job's deadline.

set -eu

if [ -z "${RUSTFS_NAMESPACE:-}" ]; then
    echo "RUSTFS_NAMESPACE must be set." >&2
    exit 1
fi
if [ -z "${RUSTFS_ACCESS_KEY:-}" ] || [ -z "${RUSTFS_SECRET_KEY:-}" ]; then
    echo "RUSTFS_ACCESS_KEY and RUSTFS_SECRET_KEY must be set." >&2
    exit 1
fi

# The tunables below exist so the script can be exercised directly (and fast)
# by .ci-scripts/test-run-rustfs-bucket-init.sh. The Job created by
# run-rustfs.sh uses the defaults; they are not plumbed through the Job
# environment.
rc_output_log="${RUSTFS_BUCKET_INIT_LOG:-/tmp/rc-output.log}"
retry_attempts="${RUSTFS_BUCKET_INIT_RETRY_ATTEMPTS:-30}"
retry_interval_seconds="${RUSTFS_BUCKET_INIT_RETRY_INTERVAL_SECONDS:-2}"
retry_deadline_seconds="${RUSTFS_BUCKET_INIT_DEADLINE_SECONDS:-240}"
command_timeout_seconds="${RUSTFS_BUCKET_INIT_COMMAND_TIMEOUT_SECONDS:-15}"

rustfs_alias=rustfs
rustfs_endpoint="http://rustfs-svc.${RUSTFS_NAMESPACE}.svc.cluster.local:9000"
bucket_name=restic-e2e

require_non_negative_integer() {
    setting_name="$1"
    setting_value="$2"
    case "${setting_value}" in
        '' | *[!0-9]*)
            echo "${setting_name} must be a non-negative integer, got '${setting_value}'." >&2
            exit 1
            ;;
    esac
    # Reject leading zeros so no shell interprets the value as octal.
    case "${setting_value}" in
        0 | [1-9]*)
            ;;
        *)
            echo "${setting_name} must not have leading zeros, got '${setting_value}'." >&2
            exit 1
            ;;
    esac
}

require_non_negative_integer RUSTFS_BUCKET_INIT_RETRY_ATTEMPTS "${retry_attempts}"
require_non_negative_integer RUSTFS_BUCKET_INIT_RETRY_INTERVAL_SECONDS "${retry_interval_seconds}"
require_non_negative_integer RUSTFS_BUCKET_INIT_DEADLINE_SECONDS "${retry_deadline_seconds}"
require_non_negative_integer RUSTFS_BUCKET_INIT_COMMAND_TIMEOUT_SECONDS "${command_timeout_seconds}"
if [ "${command_timeout_seconds}" -eq 0 ]; then
    # A zero timeout disables timeout(1)'s bound, which is the failure mode this
    # script exists to prevent.
    echo "RUSTFS_BUCKET_INIT_COMMAND_TIMEOUT_SECONDS must be greater than 0." >&2
    exit 1
fi

deadline_epoch=$(($(date +%s) + retry_deadline_seconds))

deadline_expired() {
    [ "$(date +%s)" -ge "${deadline_epoch}" ]
}

: >"${rc_output_log}"

# Run rc with a bounded runtime. Only the safe label is logged; credentials and
# full command lines are never printed.
run_rc() {
    rc_label="$1"
    shift
    rc_status=0
    timeout -k 5 "${command_timeout_seconds}" rc "$@" </dev/null \
        >>"${rc_output_log}" 2>&1 || rc_status=$?
    if [ "${rc_status}" -eq 0 ]; then
        echo "${rc_label} succeeded"
        return 0
    fi
    case "${rc_status}" in
        124 | 137 | 143)
            echo "${rc_label} timed out or was terminated after ${command_timeout_seconds}s (status ${rc_status})" >&2
            ;;
        *)
            echo "${rc_label} failed (status ${rc_status})" >&2
            ;;
    esac
    return "${rc_status}"
}

fail_bucket_init() {
    echo "$1" >&2
    echo "===== begin captured rc output (${rc_output_log}) =====" >&2
    if [ -s "${rc_output_log}" ]; then
        cat "${rc_output_log}" >&2
    else
        echo "(no output captured from rc)" >&2
    fi
    echo "===== end captured rc output =====" >&2
    exit 1
}

attempt=1
alias_configured=0
while [ "${attempt}" -le "${retry_attempts}" ]; do
    if deadline_expired; then
        break
    fi
    if run_rc "rc alias set" alias set "${rustfs_alias}" "${rustfs_endpoint}" \
        "${RUSTFS_ACCESS_KEY}" "${RUSTFS_SECRET_KEY}" \
        --region us-east-1 --bucket-lookup path; then
        alias_configured=1
        break
    fi
    if [ "${attempt}" -lt "${retry_attempts}" ] && ! deadline_expired; then
        sleep "${retry_interval_seconds}"
    fi
    attempt=$((attempt + 1))
done
if [ "${alias_configured}" -ne 1 ]; then
    fail_bucket_init "RustFS alias could not be configured after ${retry_attempts} attempts or ${retry_deadline_seconds}s."
fi

attempt=1
bucket_ready=0
while [ "${attempt}" -le "${retry_attempts}" ]; do
    if deadline_expired; then
        break
    fi
    if run_rc "rc ls ${rustfs_alias}/${bucket_name}" ls "${rustfs_alias}/${bucket_name}"; then
        bucket_ready=1
        break
    fi
    # Avoid starting another bounded command if the previous one consumed the
    # remaining budget; the Job deadline must not preempt the failure dump.
    if deadline_expired; then
        break
    fi
    if run_rc "rc mb ${rustfs_alias}/${bucket_name}" mb "${rustfs_alias}/${bucket_name}"; then
        bucket_ready=1
        break
    fi
    if [ "${attempt}" -lt "${retry_attempts}" ] && ! deadline_expired; then
        sleep "${retry_interval_seconds}"
    fi
    attempt=$((attempt + 1))
done
if [ "${bucket_ready}" -ne 1 ]; then
    fail_bucket_init "RustFS bucket '${bucket_name}' could not be found or created after ${retry_attempts} attempts or ${retry_deadline_seconds}s."
fi

echo "RustFS alias configured and bucket '${bucket_name}' is ready."
