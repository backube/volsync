#!/bin/bash
# Shared graceful-shutdown supervision for VolSync mover entrypoint scripts.
#
# Mover containers run these scripts as PID 1. The kernel does not apply
# default signal actions to PID 1, and Bash defers traps while waiting for a
# foreground command. Long-running mover processes are therefore started in
# the background via volsync_run(): Bash blocks in wait, the TERM/INT trap runs
# immediately, and the signal is forwarded to the child's process group so the
# mover can clean up before the Pod grace period expires.
#
# Helpers started with volsync_start_helper() run under a wrapper that records
# the exit status of each helper in a status file. volsync_check_helpers()
# reports critical helpers that exited, so an entrypoint can stop instead of
# waiting forever. The tail log helper is started with --noncritical. Its exit
# does not fail the check. Helper exit statuses are ignored at shutdown: the
# child started by volsync_run() has already decided the result of the run.
#
# Limitations:
# - SIGKILL cannot be trapped; nothing here can run once the Pod grace period
#   expires and the kubelet sends SIGKILL.
# - A child that ignores SIGTERM (or takes longer than the grace period to
#   clean up) is killed by the kubelet when the grace period ends.
# - Only the child started by volsync_run() (and volsync_run_cleanup()) is
#   forwarded to on a stop signal. Helpers tracked by volsync_start_helper()
#   are stopped by volsync_stop_helpers(), which runs from the EXIT trap.
#
# The entrypoint scripts allow VOLSYNC_SHUTDOWN_LIB and a few other VOLSYNC_*
# path overrides so the mover tests can run them outside the image. These are
# internal test seams: they must never be set on a real mover Job.
#
# shellcheck shell=bash

VOLSYNC_SHUTDOWN_SIGNAL=""      # set to TERM or INT when a stop signal arrives
VOLSYNC_CHILD_PID=""            # process group id of the child started by volsync_run
VOLSYNC_HELPER_PIDS=()          # process group ids of background helpers
VOLSYNC_HELPER_STATUS_FILES=()  # exit status file of each helper, same order
VOLSYNC_HELPER_CRITICAL=()      # critical flag of each helper, same order
VOLSYNC_HELPER_COMMANDS=()      # command text of each helper, same order
VOLSYNC_HELPER_STATUS_DIR=""    # directory that holds the helper status files

# Log a debugging line. Per-command logging is opt-in (DEBUG_MOVER=1) so long
# running movers do not flood the Pod logs; shutdown events are always logged.
_volsync_log_debug() {
    if [[ "${DEBUG_MOVER:-0}" == "1" ]]; then
        echo "volsync-shutdown: $*" >&2
    fi
    return 0
}

# Send a signal to a process group, falling back to the process itself if the
# group is gone or cannot be signaled. Errors are tolerated: a signal that
# arrives just as the child exits is a normal race.
_volsync_signal_group() {
    local signal="$1"
    local pgid="$2"
    kill -s "$signal" -- "-${pgid}" 2>/dev/null ||
        kill -s "$signal" "${pgid}" 2>/dev/null || true
    return 0
}

# Start a command (or shell function) in its own process group. Job control
# (set -m) is enabled only around the fork: without it, Bash puts asynchronous
# children in the shell's process group and ignores SIGINT/SIGQUIT in them.
# The caller's monitor mode is restored afterwards. The new process group id is
# returned in VOLSYNC_SPAWNED_PID.
_volsync_spawn() {
    local monitor_was_enabled=0
    case $- in
        *m*) monitor_was_enabled=1 ;;
        *) ;;
    esac

    set -m
    "$@" &
    VOLSYNC_SPAWNED_PID=$!
    if (( monitor_was_enabled == 0 )); then
        set +m
    fi
    return 0
}

# Handle a stop signal delivered to this script (PID 1 in the mover container).
# Record it so the entrypoint loops can stop retrying, then forward it to the
# process group of the currently running child so that process can clean up.
volsync_on_signal() {
    local signal="$1"
    VOLSYNC_SHUTDOWN_SIGNAL="$signal"
    echo "volsync-shutdown: received SIG${signal}" >&2
    if [[ -z "${VOLSYNC_CHILD_PID}" ]]; then
        return 0
    fi
    echo "volsync-shutdown: forwarding SIG${signal} to process group ${VOLSYNC_CHILD_PID}" >&2
    _volsync_signal_group "$signal" "${VOLSYNC_CHILD_PID}"
}

trap 'volsync_on_signal TERM' TERM
trap 'volsync_on_signal INT' INT
trap 'volsync_stop_helpers' EXIT

# Run a command (or shell function) as a supervised background job.
#
# The job runs in its own process group so a single kill can reach the whole
# pipeline. Job control (set -m) is enabled only around the fork: without it,
# Bash puts asynchronous children in the shell's process group (they would
# receive our forwarded signal only by accident) and ignores SIGINT/SIGQUIT in
# them. The caller's monitor mode is restored afterwards.
#
# Returns the child's exit status, so callers keep their existing `set -e`
# behavior.
volsync_run() {
    local rc=0
    _volsync_run 1 "$@" || rc=$?
    return "$rc"
}

# Run a short, critical cleanup command (e.g. flushing data with sync) that must
# be allowed to finish after a stop signal was received. Unlike volsync_run, an
# already-recorded signal is not forwarded to this child. A signal that arrives
# while it runs is still forwarded by the trap, so the Pod grace period remains
# the upper bound.
volsync_run_cleanup() {
    local rc=0
    _volsync_run 0 "$@" || rc=$?
    return "$rc"
}

_volsync_run() {
    local forward_existing="$1"
    shift

    if (( $# == 0 )); then
        echo "volsync-shutdown: a command is required" >&2
        return 2
    fi
    local description="$1"
    local pid rc
    _volsync_log_debug "running: $*"

    _volsync_spawn "$@"
    pid="${VOLSYNC_SPAWNED_PID}"
    VOLSYNC_CHILD_PID="$pid"

    # A stop signal may have arrived before the child was started; deliver it
    # now rather than waiting for the child to finish on its own. Cleanup
    # commands opt out of this so they can complete.
    if (( forward_existing == 1 )) && [[ -n "${VOLSYNC_SHUTDOWN_SIGNAL}" ]]; then
        echo "volsync-shutdown: forwarding SIG${VOLSYNC_SHUTDOWN_SIGNAL} to process group ${pid}" >&2
        _volsync_signal_group "${VOLSYNC_SHUTDOWN_SIGNAL}" "$pid"
    fi

    while true; do
        rc=0
        wait "$pid" || rc=$?
        # wait() returns >128 when it is interrupted by a trapped signal. If
        # the child is still alive, the signal was forwarded to it above and we
        # must keep waiting for it to finish its cleanup (and to reap it).
        if (( rc > 128 )) && kill -0 "$pid" 2>/dev/null; then
            continue
        fi
        break
    done

    # If wait was interrupted and the child exited in the window before the
    # check above, collect its real exit status. Re-waiting either returns the
    # remembered status or 127 when the job is no longer known; in the latter
    # case keep the interrupted status.
    if (( rc > 128 )); then
        local final_rc=0
        wait "$pid" 2>/dev/null || final_rc=$?
        if (( final_rc != 127 )); then
            rc=$final_rc
        fi
    fi

    VOLSYNC_CHILD_PID=""
    _volsync_log_debug "${description} exited with status ${rc}"
    return "$rc"
}

# Run a helper command and record its exit status in a status file.
# volsync_check_helpers() reads the file after the helper exits.
#
# The no-op TERM trap keeps the wrapper alive when volsync_stop_helpers()
# signals the helper process group. The wrapper can then reap the helper and
# record its status. The handler does not survive exec, so the helper still
# gets the default TERM action.
#
# Errexit is disabled for the helper command because the wrapper must record
# the status before it returns.
_volsync_helper_wrapper() {
    trap 'true' TERM
    local status_file="$1"
    shift
    local rc=0
    "$@" || rc=$?
    echo "$rc" > "${status_file}"
    return "$rc"
}

# Start a long-lived helper (e.g. stunnel, tail, diskrsync-tcp) in its own
# process group and track it so volsync_stop_helpers can terminate it.
#
# The optional --noncritical first argument marks the helper as best effort.
# An early exit of a noncritical helper does not stop the script, and
# volsync_check_helpers never fails for it. Without the flag the helper is
# critical: volsync_start_helper fails when it exits at startup, and the
# caller must run volsync_check_helpers while it waits for the transfer.
volsync_start_helper() {
    local critical=1
    if [[ "${1:-}" == "--noncritical" ]]; then
        critical=0
        shift
    fi

    if (( $# == 0 )); then
        echo "volsync-shutdown: volsync_start_helper requires a command" >&2
        return 2
    fi

    if [[ -z "${VOLSYNC_HELPER_STATUS_DIR}" ]]; then
        VOLSYNC_HELPER_STATUS_DIR=$(mktemp -d)
    fi

    local index="${#VOLSYNC_HELPER_PIDS[@]}"
    local status_file="${VOLSYNC_HELPER_STATUS_DIR}/helper-${index}.status"

    _volsync_spawn _volsync_helper_wrapper "${status_file}" "$@"
    local helper_pgid="${VOLSYNC_SPAWNED_PID}"
    VOLSYNC_HELPER_PIDS+=("${helper_pgid}")
    VOLSYNC_HELPER_STATUS_FILES+=("${status_file}")
    VOLSYNC_HELPER_CRITICAL+=("${critical}")
    VOLSYNC_HELPER_COMMANDS+=("$*")

    # Give a helper that fails at startup a moment to record its status. An
    # early exit is otherwise seen only much later, while the transfer runs.
    # The waits run through volsync_run so a stop signal in this window is
    # still forwarded to an active child.
    local _
    for _ in 1 2 3 4; do
        if [[ -e "${status_file}" ]] || volsync_shutdown_requested; then
            break
        fi
        volsync_run sleep 0.05 || true
    done

    if [[ -e "${status_file}" ]]; then
        local rc
        rc=$(cat "${status_file}" 2>/dev/null || true)
        if (( critical == 1 )); then
            echo "volsync-shutdown: helper failed to start: $* (status ${rc:-unknown})" >&2
            return 1
        fi
        echo "volsync-shutdown: noncritical helper exited: $* (status ${rc:-unknown})" >&2
        return 0
    fi

    echo "volsync-shutdown: started helper: $* (process group ${helper_pgid})" >&2
    return 0
}

# Return 1 when a critical helper has exited. Return 0 while every critical
# helper is alive. A helper is alive while its status file is absent. Nothing
# is printed while all critical helpers are alive.
volsync_check_helpers() {
    local i
    local rc=0
    local helper_rc
    local status_file
    for i in "${!VOLSYNC_HELPER_PIDS[@]}"; do
        if (( VOLSYNC_HELPER_CRITICAL[i] == 0 )); then
            continue
        fi
        status_file="${VOLSYNC_HELPER_STATUS_FILES[i]}"
        if [[ ! -e "${status_file}" ]]; then
            continue
        fi
        helper_rc=$(cat "${status_file}" 2>/dev/null || true)
        echo "volsync-shutdown: helper exited unexpectedly: ${VOLSYNC_HELPER_COMMANDS[i]} (status ${helper_rc:-unknown})" >&2
        rc=1
    done
    return "${rc}"
}

# Terminate and reap every tracked helper. Safe to call when the list is empty
# and safe to call more than once (the EXIT trap runs it after any explicit
# call). The list is cleared first so a second call is a no-op, and so a helper
# that fails to stop cannot block the script from exiting cleanly.
#
# Helper exit statuses are ignored here on purpose. The transfer child started
# by volsync_run() has already decided the result of the run.
volsync_stop_helpers() {
    local pid
    local -a pids

    if (( ${#VOLSYNC_HELPER_PIDS[@]} == 0 )); then
        return 0
    fi

    pids=("${VOLSYNC_HELPER_PIDS[@]}")
    VOLSYNC_HELPER_PIDS=()
    VOLSYNC_HELPER_STATUS_FILES=()
    VOLSYNC_HELPER_CRITICAL=()
    VOLSYNC_HELPER_COMMANDS=()

    for pid in "${pids[@]}"; do
        echo "volsync-shutdown: stopping helper process group ${pid}" >&2
        _volsync_signal_group TERM "$pid"
    done
    # Helpers that ignore TERM are bounded by the Pod grace period: the kubelet
    # sends SIGKILL when it expires. stunnel, tail and diskrsync-tcp all exit on
    # TERM, so this normally returns immediately.
    for pid in "${pids[@]}"; do
        wait "$pid" 2>/dev/null || true
    done
    if [[ -n "${VOLSYNC_HELPER_STATUS_DIR}" ]]; then
        rm -rf "${VOLSYNC_HELPER_STATUS_DIR}"
        VOLSYNC_HELPER_STATUS_DIR=""
    fi
    return 0
}

# True (rc 0) once a TERM or INT has been received.
volsync_shutdown_requested() {
    if [[ -n "${VOLSYNC_SHUTDOWN_SIGNAL}" ]]; then
        return 0
    fi
    return 1
}
