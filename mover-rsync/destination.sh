#! /bin/bash

set -e -o pipefail

# shellcheck source=/dev/null
source "${VOLSYNC_SHUTDOWN_LIB:-/mover-common/shutdown.sh}"

echo "VolSync rsync container version: ${version:-unknown}"

SCRIPT_FULLPATH="$(realpath "$0")"
SCRIPT="$(basename "$SCRIPT_FULLPATH")"
SCRIPT_DIR="$(dirname "$SCRIPT_FULLPATH")"

# Do not do this debug mover code if this is already the
# mover script copy in /tmp
if [[ $DEBUG_MOVER -eq 1 && "$SCRIPT_DIR" != "/tmp" ]]; then
  MOVER_SCRIPT_COPY="/tmp/$SCRIPT"
  cp "$SCRIPT_FULLPATH" "$MOVER_SCRIPT_COPY"

  END_DEBUG_FILE="/tmp/exit-debug-if-removed"
  touch $END_DEBUG_FILE

  echo ""
  echo "##################################################################"
  echo "DEBUG_MOVER is enabled, this pod will sleep indefinitely."
  echo ""
  echo "The mover script that would normally run has been copied to"
  echo "$MOVER_SCRIPT_COPY".
  echo ""
  echo "To debug, you can modify this file and run it with:"
  echo "$MOVER_SCRIPT_COPY" "$@"
  echo ""
  echo "If you wish to exit this pod after debugging, delete the"
  echo "file $END_DEBUG_FILE from the system."
  echo "##################################################################"

  # Wait for user to delete the file before exiting
  while [[ -f "${END_DEBUG_FILE}" && -z "${VOLSYNC_SHUTDOWN_SIGNAL}" ]]; do
    volsync_run sleep 10
  done

  echo ""
  echo "##################################################################"
  echo "Debug done, exiting."
  echo "##################################################################"
  volsync_run sleep 2
  exit 0
fi

# Allow source's key to access, but restrict what it can do.
# The VOLSYNC_* path overrides exist so the mover shell tests can run against a
# sandbox; the mover Job always uses the image defaults.
KEY_DIR="${VOLSYNC_KEY_DIR:-/keys}"
SSHD_BIN="${VOLSYNC_SSHD_BIN:-/usr/sbin/sshd}"
mkdir -p ~/.ssh
chmod 700 ~/.ssh
echo "command=\"/mover-rsync/destination-command.sh\",restrict $(<"${KEY_DIR}/source.pub")" > ~/.ssh/authorized_keys

MOUNT_PATH="${VOLSYNC_DATA_DIR:-/data}"
VOLUME_MODE="filesystem"
if test -b /dev/block; then
    VOLUME_MODE=block
    MOUNT_PATH="/dev/block"
fi
echo "Destination PVC volumeMode is $VOLUME_MODE"

# Wait for incoming rsync transfer
echo "Waiting for connection..."
rm -f /var/run/nologin
# Do not use exec: the exit-code logic below must still run after sshd exits.
# A non-zero sshd exit is fatal unless it was caused by a forwarded TERM, in
# which case we still want to sync the volume and report the interruption.
sshd_rc=0
volsync_run "$SSHD_BIN" -D -e -p 8022 || sshd_rc=$?
if [[ ${sshd_rc} -ne 0 && -z "${VOLSYNC_SHUTDOWN_SIGNAL}" ]]; then
    echo "sshd exited with status ${sshd_rc}"
    exit "${sshd_rc}"
fi

# When sshd exits, need to return the proper exit code from the rsync operation
CODE=255
if [[ -e /tmp/exit_code ]]; then
    CODE_IN="$(</tmp/exit_code)"
    if [[ $CODE_IN =~ ^[0-9]+$ ]]; then
        CODE="$CODE_IN"
    fi
elif [[ -n "${VOLSYNC_SHUTDOWN_SIGNAL}" ]]; then
    echo "sshd stopped by SIG${VOLSYNC_SHUTDOWN_SIGNAL} before the transfer completed"
fi
volsync_run_cleanup sync -f "${MOUNT_PATH}"
echo "Exiting... Exit code: $CODE"
exit "$CODE"
