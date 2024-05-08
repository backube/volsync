#! /bin/bash

set -e -o pipefail

DESTINATION_PORT="${DESTINATION_PORT:-8000}"
if [[ -z "$DESTINATION_ADDRESS" ]]; then
    echo "Remote host address must be provided in DESTINATION_ADDRESS"
    exit 1
fi

STUNNEL_CONF=/tmp/stunnel-client.conf
STUNNEL_PID_FILE=/tmp/stunnel-client.pid
PSK_FILE=/keys/psk.txt
STUNNEL_LISTEN_PORT=9000
SOURCE="/data"
BLOCK_SOURCE="/dev/block"

SCRIPT="$(realpath "$0")"
SCRIPT_DIR="$(dirname "$SCRIPT")"
cd "$SCRIPT_DIR"

DEBUG_FILE_BASE=/tmp/run_command
DEBUG_CMD_COUNT=0
function run_command() {
  DEBUG_CMD_COUNT=$((DEBUG_CMD_COUNT + 1))
  DEBUG_FILE="${DEBUG_FILE_BASE}-${DEBUG_CMD_COUNT}"

  if [[ $DEBUG_MOVER -eq 1 ]]; then
    {
      echo "Next command: "
      echo ""
      echo "$@"
      echo ""
      echo "Run the command manually, then to continue, delete this file"
    } > ${DEBUG_FILE}

    echo ""
    echo "##################################################################"
    echo "DEBUG_MOVER is enabled, commands will need to be run manually"
    echo "The command that would run next is shown in file ${DEBUG_FILE}"
    echo "To continue this script, delete file ${DEBUG_FILE}"
    echo "##################################################################"

    # Wait for user to delete the file before proceeding
    while [[ -f "${DEBUG_FILE}" ]]; do
      sleep 5
    done
    echo ""
    echo "Continuing ..."
  else
    echo Running command "$@"
    "$@"
  fi
}

# shellcheck disable=SC2317  # It's reachable due to the TRAP
function stop_stunnel() {
    ## Terminate stunnel
    kill -TERM "$(<"$STUNNEL_PID_FILE")"
}

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
foreground = no
output = /dev/stdout
pid = $STUNNEL_PID_FILE
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
foreground = no
output = /dev/stdout
pid = $STUNNEL_PID_FILE
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
run_command stunnel "$STUNNEL_CONF"
run_command trap stop_stunnel EXIT

# Sync files
START_TIME=$SECONDS
MAX_RETRIES=5
RETRY=0
DELAY=2
FACTOR=2
rc=1
set +e  # Don't exit on command failure
echo "Syncing data to ${DESTINATION_ADDRESS}:${DESTINATION_PORT} ..."
while [[ $rc -ne 0 && $RETRY -lt $MAX_RETRIES ]]; do
    RETRY=$(( RETRY + 1 ))
    if test -b $BLOCK_SOURCE; then
      echo "calling diskrsync-tcp $BLOCK_SOURCE --source --target-address 127.0.0.1 --port $STUNNEL_LISTEN_PORT"
      run_command /diskrsync-tcp $BLOCK_SOURCE --source --target-address 127.0.0.1 --port $STUNNEL_LISTEN_PORT
      rc=$?
    else
        shopt -s dotglob  # Make * include dotfiles
        if [[ -n "$(ls -A -- ${SOURCE}/*)" ]]; then
            # 1st run preserves as much as possible, but excludes the root directory
            run_command rsync -aAhHSxz --exclude=lost+found --itemize-changes --info=stats2,misc2 ${SOURCE}/* rsync://127.0.0.1:$STUNNEL_LISTEN_PORT/data
        else
            echo "Skipping sync of empty source directory"
        fi
        rc_a=$?
        shopt -u dotglob  # Back to default * behavior

        # To delete extra files, must sync at the directory-level, but need to avoid
        # trying to modify the directory itself. This pass will only delete files
        # that exist on the destination but not on the source, not make updates.
        run_command rsync -rx --exclude=lost+found --ignore-existing --ignore-non-existing --delete --itemize-changes --info=stats2,misc2 ${SOURCE}/ rsync://127.0.0.1:$STUNNEL_LISTEN_PORT/data
        rc_b=$?
        rc=$(( rc_a * 100 + rc_b ))
    fi
    if [[ $rc -ne 0 ]]; then
        echo "Syncronization failed. Retrying in $DELAY seconds. Retry ${RETRY}/${MAX_RETRIES}."
        sleep $DELAY
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
        run_command rsync "$SCRIPT" rsync://127.0.0.1:$STUNNEL_LISTEN_PORT/control/complete
        echo "...done"
        sleep 5  # Give time for the remote to shut down
    else
        echo "Synchronization failed. rsync returned: $rc"
    fi
fi

exit $rc
