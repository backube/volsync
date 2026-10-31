#!/usr/bin/env bash
#
# Container-level regression test for mover stop-signal forwarding (issue #2171).
#
# When a mover Pod is deleted, Kubernetes sends SIGTERM to the container's
# PID 1 only (not to a process group). The mover entrypoints are expected to
# forward that signal to the mover process (mover-common/shutdown.sh), so that
# Restic can clean up and release its repository lock well inside the mover
# grace period (MoverTerminationGracePeriodSeconds, 120s).
#
# `docker kill --signal TERM <container>` reproduces the Kubernetes behavior:
# the runtime sends the signal to container PID 1 and nothing else. This test
# starts the real mover image, runs a Restic backup large enough to be holding
# the repository lock when the signal arrives, and then verifies that:
#   - the entrypoint forwarded the signal to Restic (log line),
#   - the container exited before the 120s grace period expired,
#   - Restic released its lock (no files left in <repo>/locks),
#   - a fresh backup can take the lock again (repo is not left locked),
#   - `restic check` reports a healthy repository.
#
# Usage:
#   ./.ci-scripts/test-mover-shutdown-container.sh <image>
#
# Environment:
#   CONTAINER_RUNTIME  docker or podman (default: docker, podman if docker is
#                      not installed)
#   TEST_DATA_SIZE_MB  amount of incompressible fixture data to back up
#                      (default: 256)
#
# Example:
#   make docker-build IMG=localhost/volsync-shutdown-test:dev
#   ./.ci-scripts/test-mover-shutdown-container.sh localhost/volsync-shutdown-test:dev
#
# shortcut: the fixture size assumes the first backup outlasts the lock poll.
# If a future Restic finishes the backup while this test is still polling for
# the lock (or CI raises the CPU allowance), raise TEST_DATA_SIZE_MB.

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
readonly REPO_ROOT

# The product default grace period for mover Pods (see
# internal/controller/mover/shutdown.go). The test fails if shutdown needs it.
readonly GRACE_PERIOD_SECONDS=120
readonly LOCK_WAIT_SECONDS=60
readonly POLL_INTERVAL_SECONDS=0.1
# Restic handles SIGTERM, cleans up, and exits with its documented
# "interrupted" status. The entrypoint's `set -e` exits with that same status.
# An exit code of 143 (killed by the signal's default action, no cleanup) is
# therefore a failure.
readonly RESTIC_EXIT_INTERRUPTED=130
readonly SHUTDOWN_FORWARD_LOG="volsync-shutdown: forwarding SIGTERM"
readonly RESTIC_CLEANUP_LOG="signal terminated received, cleaning up"

fail() {
    printf 'FAIL: %s\n' "$*" >&2
    exit 1
}

usage() {
    printf 'usage: %s <image>\n' "${BASH_SOURCE[0]}" >&2
}

if (( $# != 1 )) || [[ -z "${1}" ]]; then
    usage
    exit 2
fi
readonly IMAGE="$1"

RUNTIME=""
detect_runtime() {
    if [[ -n "${CONTAINER_RUNTIME:-}" ]]; then
        command -v "${CONTAINER_RUNTIME}" >/dev/null 2>&1 ||
            fail "CONTAINER_RUNTIME='${CONTAINER_RUNTIME}' was not found in PATH"
        RUNTIME="${CONTAINER_RUNTIME}"
    elif command -v docker >/dev/null 2>&1; then
        RUNTIME="docker"
    elif command -v podman >/dev/null 2>&1; then
        RUNTIME="podman"
    else
        fail "neither docker nor podman is available; set CONTAINER_RUNTIME"
    fi
}
detect_runtime

if ! "${RUNTIME}" image inspect "${IMAGE}" >/dev/null 2>&1; then
    fail "container image '${IMAGE}' is not available locally; build it first, e.g.:"$'\n'"  (cd '${REPO_ROOT}' && make docker-build IMG=${IMAGE})"
fi

TEST_DATA_SIZE_MB="${TEST_DATA_SIZE_MB:-256}"
if ! [[ "${TEST_DATA_SIZE_MB}" =~ ^[1-9][0-9]*$ ]]; then
    fail "TEST_DATA_SIZE_MB must be a positive integer, got '${TEST_DATA_SIZE_MB}'"
fi

TEST_TMP="$(mktemp -d)"
readonly TEST_TMP
REPO_DIR="${TEST_TMP}/repo"
DATA_DIR="${TEST_TMP}/data"
CACHE_DIR="${TEST_TMP}/cache"
CONTAINER_PREFIX="volsync-shutdown-test-$$"
CONTAINER_LOG="${TEST_TMP}/container.log"
CLEANUP_CONTAINERS=()

# Remove every container this run created plus the fixture data. Containers are
# not started with --rm: they must outlive their exit so the exit code and the
# logs can be inspected, which the runtime discards once a container is
# auto-removed.
cleanup() {
    if (( ${#CLEANUP_CONTAINERS[@]} > 0 )); then
        "${RUNTIME}" rm -f "${CLEANUP_CONTAINERS[@]}" >/dev/null 2>&1 || true
    fi
    rm -rf "${TEST_TMP}"
}
trap cleanup EXIT

mkdir -p "${REPO_DIR}" "${DATA_DIR}" "${CACHE_DIR}"

# Restic's local backend needs a directory (it creates the layout itself).
# The fixture must be incompressible: sparse files are backed up too quickly to
# observe the lock.
printf '=== Creating %s MiB of incompressible test data ===\n' "${TEST_DATA_SIZE_MB}"
TEST_DATA_FILE_BYTES=$(( TEST_DATA_SIZE_MB * 1024 * 1024 / 4 ))
for file_index in 1 2 3 4; do
    head -c "${TEST_DATA_FILE_BYTES}" /dev/urandom >"${DATA_DIR}/data-${file_index}.bin"
done

# `:z` relabels the bind mounts for SELinux hosts (a no-op when SELinux is
# disabled or absent) so the same mounts work for docker and podman.
COMMON_ARGS=(
    --env PRIVILEGED_MOVER=1
    --env RESTIC_REPOSITORY=/repo
    --env RESTIC_PASSWORD=test-password
    --env RESTIC_CACHE_DIR=/cache
    --env DATA_DIR=/data
    --volume "${REPO_DIR}:/repo:z"
    --volume "${DATA_DIR}:/data:z"
    --volume "${CACHE_DIR}:/cache:z"
)

# Rootful docker would create the repository files as root, and the sub-
# directories it creates are not writable by the caller, so the cleanup trap
# could not delete them. Run as the calling user instead; Restic needs no
# privileges and the signal handling under test is unaffected. Rootless podman
# already maps container root to the calling user.
if [[ "${RUNTIME}" == "docker" ]]; then
    COMMON_ARGS+=(--user "$(id -u):$(id -g)")
fi

container_is_running() {
    local running=""
    running="$("${RUNTIME}" inspect -f '{{.State.Running}}' "${1}" 2>/dev/null)" || running=""
    [[ "${running}" == "true" ]]
}

# Restic writes a lock file to <repo>/locks before it starts scanning; it
# creates the file under a temporary name and renames it, so ignore temp files.
held_lock_file() {
    find "${REPO_DIR}/locks" -maxdepth 1 -type f ! -name '*.tmp' -print -quit 2>/dev/null || true
}

dump_logs() {
    "${RUNTIME}" logs "${1}" >"${CONTAINER_LOG}" 2>&1 || true
    printf '=== Container logs (%s) ===\n' "${1}" >&2
    cat "${CONTAINER_LOG}" >&2 || true
}

# Poll until Restic holds the repository lock. Fails if the container exits
# first, which means the fixture was too small to observe the backup.
wait_for_restic_lock() {
    local deadline=$(( SECONDS + LOCK_WAIT_SECONDS ))
    local lock_file=""
    while (( SECONDS < deadline )); do
        lock_file="$(held_lock_file)"
        if [[ -n "${lock_file}" ]]; then
            printf '=== Restic holds the repository lock: %s ===\n' "$(basename "${lock_file}")"
            return 0
        fi
        if ! container_is_running "${1}"; then
            return 1
        fi
        sleep "${POLL_INTERVAL_SECONDS}"
    done
    return 1
}

# Poll until the container is no longer running, up to the grace period.
wait_for_container_exit() {
    local deadline=$(( SECONDS + GRACE_PERIOD_SECONDS ))
    while (( SECONDS < deadline )); do
        if ! container_is_running "${1}"; then
            return 0
        fi
        sleep "${POLL_INTERVAL_SECONDS}"
    done
    return 1
}

require_log_line() {
    local pattern="$1"
    if ! grep -Fq -- "${pattern}" "${CONTAINER_LOG}"; then
        fail "container logs do not contain: ${pattern}"
    fi
    printf '    %s\n' "$(grep -Fm1 -- "${pattern}" "${CONTAINER_LOG}")"
}

printf '=== Starting %s backup as container PID 1 (%s) ===\n' "${RUNTIME}" "${IMAGE}"
MAIN_CONTAINER="${CONTAINER_PREFIX}-backup"
CLEANUP_CONTAINERS+=("${MAIN_CONTAINER}")
"${RUNTIME}" run --detach --name "${MAIN_CONTAINER}" --cpus 1 \
    "${COMMON_ARGS[@]}" \
    --entrypoint /bin/bash "${IMAGE}" /mover-restic/entry.sh backup >/dev/null

if ! wait_for_restic_lock "${MAIN_CONTAINER}"; then
    dump_logs "${MAIN_CONTAINER}"
    fail "Restic did not hold a repository lock within ${LOCK_WAIT_SECONDS}s (the container exited first, or a ${TEST_DATA_SIZE_MB} MiB fixture is too small); see the container logs above"
fi

# SIGTERM to container PID 1 only, exactly like the kubelet does when the Pod
# grace period starts. `docker stop` is not used because it would also apply
# the runtime's own stop timeout.
printf '=== Sending SIGTERM to container PID 1 ===\n'
if ! container_is_running "${MAIN_CONTAINER}"; then
    dump_logs "${MAIN_CONTAINER}"
    fail "the backup finished before SIGTERM could be delivered (the ${TEST_DATA_SIZE_MB} MiB fixture is too small); raise TEST_DATA_SIZE_MB"
fi
START_NANOS="$(date +%s%N)"
"${RUNTIME}" kill --signal TERM "${MAIN_CONTAINER}" >/dev/null

if ! wait_for_container_exit "${MAIN_CONTAINER}"; then
    dump_logs "${MAIN_CONTAINER}"
    fail "container was still running ${GRACE_PERIOD_SECONDS}s after SIGTERM"
fi
END_NANOS="$(date +%s%N)"
ELAPSED_MS=$(( (END_NANOS - START_NANOS) / 1000000 ))

EXIT_CODE="$("${RUNTIME}" inspect -f '{{.State.ExitCode}}' "${MAIN_CONTAINER}" 2>/dev/null)" ||
    fail "could not read the exit code of ${MAIN_CONTAINER}"
"${RUNTIME}" logs "${MAIN_CONTAINER}" >"${CONTAINER_LOG}" 2>&1 || true

(( ELAPSED_MS < GRACE_PERIOD_SECONDS * 1000 )) ||
    fail "shutdown took $((ELAPSED_MS / 1000))s, which is not less than the ${GRACE_PERIOD_SECONDS}s grace period"

if [[ "${EXIT_CODE}" != "${RESTIC_EXIT_INTERRUPTED}" ]]; then
    cat "${CONTAINER_LOG}" >&2 || true
    fail "unexpected exit code ${EXIT_CODE}, expected ${RESTIC_EXIT_INTERRUPTED} (Restic's 'interrupted' status): 0 means the stop signal never reached Restic, 143 means Restic was killed without cleaning up"
fi

STALE_LOCKS="$(held_lock_file)"
if [[ -n "${STALE_LOCKS}" ]]; then
    printf '=== Remaining lock files ===\n' >&2
    find "${REPO_DIR}/locks" -maxdepth 1 -type f >&2 || true
    fail "Restic did not release its repository lock"
fi

printf '=== Signal path in the container logs ===\n'
require_log_line "${SHUTDOWN_FORWARD_LOG}"
require_log_line "${RESTIC_CLEANUP_LOG}"

# A second backup proves the repository is unlocked (no stale lock) and that the
# interrupted backup did not corrupt it.
printf '=== Verifying that a new backup can run ===\n'
SECOND_CONTAINER="${CONTAINER_PREFIX}-backup-after-stop"
CLEANUP_CONTAINERS+=("${SECOND_CONTAINER}")
SECOND_LOG="${TEST_TMP}/second-backup.log"
if ! timeout "${GRACE_PERIOD_SECONDS}s" "${RUNTIME}" run --rm --name "${SECOND_CONTAINER}" \
    "${COMMON_ARGS[@]}" \
    --entrypoint /bin/bash "${IMAGE}" /mover-restic/entry.sh backup >"${SECOND_LOG}" 2>&1; then
    printf '=== Second backup logs ===\n' >&2
    cat "${SECOND_LOG}" >&2
    fail "a second backup failed (stale lock or damaged repository)"
fi

printf '=== Running restic check ===\n'
CHECK_CONTAINER="${CONTAINER_PREFIX}-check"
CLEANUP_CONTAINERS+=("${CHECK_CONTAINER}")
CHECK_LOG="${TEST_TMP}/restic-check.log"
if ! timeout "${GRACE_PERIOD_SECONDS}s" "${RUNTIME}" run --rm --name "${CHECK_CONTAINER}" \
    "${COMMON_ARGS[@]}" \
    --entrypoint /usr/local/bin/restic "${IMAGE}" check >"${CHECK_LOG}" 2>&1; then
    printf '=== restic check logs ===\n' >&2
    cat "${CHECK_LOG}" >&2
    fail "restic check reported problems with the repository"
fi

printf '\nPASS: mover shutdown signal forwarding\n'
printf '  image:             %s\n' "${IMAGE}"
printf '  TERM to exit:      %d.%01ds (grace period %ds)\n' \
    "$(( ELAPSED_MS / 1000 ))" "$(( (ELAPSED_MS % 1000) / 100 ))" "${GRACE_PERIOD_SECONDS}"
printf '  container exit:    %s\n' "${EXIT_CODE}"
printf '  lock released:     yes\n'
printf '  follow-up backup:  exit 0\n'
printf '  restic check:      exit 0\n'
