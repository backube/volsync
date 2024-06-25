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
stunnel "$STUNNEL_CONF"
trap stop_stunnel EXIT

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
      /diskrsync-tcp $BLOCK_SOURCE --source --target-address 127.0.0.1 --port $STUNNEL_LISTEN_PORT
      rc=$?
    else
        # Find all files/dirs at root of pvc, prepend / to each (rsync will use SOURCE as the base dir for these files)
        find "${SOURCE}" -mindepth 1 -maxdepth 1 -printf '/%P\n' > /tmp/filelist.txt
        if [[ -s /tmp/filelist.txt ]]; then
            # 1st run preserves as much as possible, but excludes the root directory
            rsync -aAhHSxz -r --exclude=lost+found --itemize-changes --info=stats2,misc2 --files-from=/tmp/filelist.txt ${SOURCE}/ rsync://127.0.0.1:$STUNNEL_LISTEN_PORT/data
        else
            echo "Skipping sync of empty source directory"
        fi
        rc_a=$?

        # To delete extra files, must sync at the directory-level, but need to avoid
        # trying to modify the directory itself. This pass will only delete files
        # that exist on the destination but not on the source, not make updates.
        rsync -rx --exclude=lost+found --ignore-existing --ignore-non-existing --delete --itemize-changes --info=stats2,misc2 ${SOURCE}/ rsync://127.0.0.1:$STUNNEL_LISTEN_PORT/data
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
        rsync "$SCRIPT" rsync://127.0.0.1:$STUNNEL_LISTEN_PORT/control/complete
        echo "...done"
        sleep 5  # Give time for the remote to shut down
    else
        echo "Synchronization failed. rsync returned: $rc"
    fi
fi

exit $rc
