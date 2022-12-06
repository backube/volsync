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
SOURCE=/data

SCRIPT="$(realpath "$0")"
SCRIPT_DIR="$(dirname "$SCRIPT")"
cd "$SCRIPT_DIR"

function stop_stunnel() {
    ## Terminate stunnel
    kill -TERM "$(<"$STUNNEL_PID_FILE")"
}

if [[ ! -r $PSK_FILE ]]; then
    echo "ERROR: Pre-shared key not found - $PSK_FILE"
    exit 1
fi

if [[ ! -d "$SOURCE" ]]; then
    echo "ERROR: PVC not mounted at $SOURCE"
    exit 1
fi


cat - > "$STUNNEL_CONF" <<STUNNEL_CONF
; Global options
debug = debug
foreground = no
output = /dev/stdout
pid = $STUNNEL_PID_FILE
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
    shopt -s dotglob  # Make * include dotfiles
    if [[ -n "$(ls -A -- ${SOURCE}/*)" ]]; then
        # 1st run preserves as much as possible, but excludes the root directory
        rsync -aAhHSxz --exclude=lost+found --itemize-changes --info=stats2,misc2 ${SOURCE}/* rsync://127.0.0.1:$STUNNEL_LISTEN_PORT/data
    else
        echo "Skipping sync of empty source directory"
    fi
    rc_a=$?
    shopt -u dotglob  # Back to default * behavior

    # To delete extra files, must sync at the directory-level, but need to avoid
    # trying to modify the directory itself. This pass will only delete files
    # that exist on the destination but not on the source, not make updates.
    rsync -rx --exclude=lost+found --ignore-existing --ignore-non-existing --delete --itemize-changes --info=stats2,misc2 ${SOURCE}/ rsync://127.0.0.1:$STUNNEL_LISTEN_PORT/data
    rc_b=$?

    rc=$(( rc_a * 100 + rc_b ))
    if [[ $rc -ne 0 ]]; then
        echo "Syncronization failed. Retrying in $DELAY seconds. Retry ${RETRY}/${MAX_RETRIES}."
        sleep $DELAY
        DELAY=$(( DELAY * FACTOR ))
    fi
done
set -e  # Exit on command failure

echo "rsync completed in $(( SECONDS - START_TIME ))s"

if [[ $rc -eq 0 ]]; then
    # Tell server to shutdown. Actual file contents don't matter
    echo "Sending shutdown to remote..."
    rsync "$SCRIPT" rsync://127.0.0.1:$STUNNEL_LISTEN_PORT/control/complete
    echo "...done"
else
    echo "Synchronization failed. rsync returned: $rc"
fi

exit $rc
