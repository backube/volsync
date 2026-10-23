#! /bin/bash

set -e -o pipefail

# shellcheck source=/dev/null
source "${VOLSYNC_SHUTDOWN_LIB:-/mover-common/shutdown.sh}"

DESTINATION_PORT="${DESTINATION_PORT:-8000}"
if [[ -z "$DESTINATION_ADDRESS" ]]; then
    echo "Remote host address must be provided in DESTINATION_ADDRESS"
    exit 1
fi

STUNNEL_CONF=/tmp/stunnel-client.conf
# The VOLSYNC_* path overrides exist so the mover shell tests can run against a
# sandbox; the mover Job always uses the image defaults.
PSK_FILE="${VOLSYNC_KEY_DIR:-/keys}/psk.txt"
DISKRSYNC_TCP_BIN="${VOLSYNC_DISKRSYNC_TCP_BIN:-/diskrsync-tcp}"
STUNNEL_LISTEN_PORT=9000
SOURCE="${VOLSYNC_DATA_DIR:-/data}"
BLOCK_SOURCE="/dev/block"

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

cd "$SCRIPT_DIR"

if [[ ! -r $PSK_FILE ]]; then
    echo "ERROR: Pre-shared key not found - $PSK_FILE"
    exit 1
fi

if [[ ! -d $SOURCE ]] && ! test -b $BLOCK_SOURCE; then
    echo "ERROR: source location not found"
    exit 1
fi

if ! test -b $BLOCK_SOURCE; then
    echo "Source PVC volumeMode is filesystem"

    cat - > "$STUNNEL_CONF" <<STUNNEL_CONF
; Global options
debug = debug
foreground = yes
output = /dev/stdout
socket = l:SO_KEEPALIVE=1
socket = l:TCP_KEEPIDLE=180
socket = r:SO_KEEPALIVE=1
socket = r:TCP_KEEPIDLE=180
syslog = no

[rsync]
ciphers = PSK
PSKsecrets = $PSK_FILE
; Port to listen for incoming connection from rsync
accept = 127.0.0.1:$STUNNEL_LISTEN_PORT
; We are the client
client = yes
connect = $DESTINATION_ADDRESS:$DESTINATION_PORT
STUNNEL_CONF

##############################
## Print version information
rsync --version
else
    echo "Source PVC volumeMode is block"

    cat - > "$STUNNEL_CONF" <<STUNNEL_CONF
; Global options
debug = debug
foreground = yes
output = /dev/stdout
socket = l:SO_KEEPALIVE=1
socket = l:TCP_KEEPIDLE=180
socket = r:SO_KEEPALIVE=1
socket = r:TCP_KEEPIDLE=180
syslog = no

[diskrsync]
ciphers = PSK
PSKsecrets = $PSK_FILE
; Port to listen for incoming connection from diskrsync
accept = 127.0.0.1:$STUNNEL_LISTEN_PORT
; We are the client
client = yes
connect = $DESTINATION_ADDRESS:$DESTINATION_PORT
STUNNEL_CONF
fi
stunnel -version "$STUNNEL_CONF"

##############################
## Start stunnel to wait for incoming connections
volsync_start_helper stunnel "$STUNNEL_CONF"

# First rsync pass, kept in a function so volsync_run supervises the whole
# rsync|tee pipeline as one process group.
# shellcheck disable=SC2329  # invoked indirectly via volsync_run
function run_rsync_transfer {
    rsync -aAhHSxz -r --exclude=lost+found --itemize-changes --info=stats2,misc2 --files-from=/tmp/filelist.txt "${SOURCE}"/ rsync://127.0.0.1:"$STUNNEL_LISTEN_PORT"/data | tee /tmp/rsync-full.log
}

# Sync files
START_TIME=$SECONDS
MAX_RETRIES=5
RETRY=0
DELAY=2
FACTOR=2
rc=1
set +e  # Don't exit on command failure
echo "Syncing data to ${DESTINATION_ADDRESS}:${DESTINATION_PORT} ..."
while [[ $rc -ne 0 && $RETRY -lt $MAX_RETRIES && -z "${VOLSYNC_SHUTDOWN_SIGNAL}" ]]; do
    RETRY=$(( RETRY + 1 ))
    if test -b $BLOCK_SOURCE; then
      echo "calling diskrsync-tcp $BLOCK_SOURCE --source --target-address 127.0.0.1 --port $STUNNEL_LISTEN_PORT"
      volsync_run "$DISKRSYNC_TCP_BIN" $BLOCK_SOURCE --source --target-address 127.0.0.1 --port $STUNNEL_LISTEN_PORT
      rc=$?
    else
        # Find all files/dirs at root of pvc, prepend / to each (rsync will use SOURCE as the base dir for these files)
        find "${SOURCE}" -mindepth 1 -maxdepth 1 -printf '/%P\n' > /tmp/filelist.txt
        if [[ -s /tmp/filelist.txt ]]; then
            # 1st run preserves as much as possible, but excludes the root directory
            volsync_run run_rsync_transfer
            rc_a=$?

            if [[ $rc_a -eq 0 ]]; then
                # Symlinks - if any symlinks were created or modified, create a control file containing the list of symlinks
                # Symlinks in rsync --itemize-changes output are prefixed with 'cL' when sent to rsync daemon
                grep -E '^cL' /tmp/rsync-full.log | awk -F' -> ' '{print $1}' | sed 's/^[^ ]* //' >> /tmp/symlink-munging-file
                if [[ -s /tmp/symlink-munging-file ]]; then
                    echo "Symlinks were created or modified, sending symlink munging file to destination..."
                    volsync_run rsync /tmp/symlink-munging-file rsync://127.0.0.1:$STUNNEL_LISTEN_PORT/control/symlink-munging-file
                    rc_a=$?
                fi
            fi
        else
            echo "Skipping sync of empty source directory"
            rc_a=0
        fi

        # To delete extra files, must sync at the directory-level, but need to avoid
        # trying to modify the directory itself. This pass will only delete files
        # that exist on the destination but not on the source, not make updates.
        if [[ -z "${VOLSYNC_SHUTDOWN_SIGNAL}" ]]; then
            volsync_run rsync -rx --exclude=lost+found --ignore-existing --ignore-non-existing --delete --itemize-changes --info=stats2,misc2 "${SOURCE}"/ rsync://127.0.0.1:$STUNNEL_LISTEN_PORT/data
            rc_b=$?
            rc=$(( rc_a * 100 + rc_b ))
        else
            # Do not start another transfer pass once shutdown has been requested.
            rc=$rc_a
        fi
    fi
    if [[ $rc -ne 0 && -z "${VOLSYNC_SHUTDOWN_SIGNAL}" ]]; then
        echo "Syncronization failed. Retrying in $DELAY seconds. Retry ${RETRY}/${MAX_RETRIES}."
        volsync_run sleep "$DELAY"
        DELAY=$(( DELAY * FACTOR ))
    fi
done
set -e  # Exit on command failure

if test -b $BLOCK_SOURCE; then
    echo "diskrsync completed in $(( SECONDS - START_TIME ))s"
else
    echo "rsync completed in $(( SECONDS - START_TIME ))s"

    if [[ $rc -eq 0 ]]; then
        # Tell server to shutdown. Actual file contents don't matter
        echo "Sending shutdown to remote..."
        volsync_run rsync "$SCRIPT_FULLPATH" rsync://127.0.0.1:$STUNNEL_LISTEN_PORT/control/complete
        echo "...done"
        volsync_run sleep 5  # Give time for the remote to shut down
    else
        echo "Synchronization failed. rsync returned: $rc"
    fi
fi

exit $rc
