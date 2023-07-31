#! /bin/bash

set -e -o pipefail

RSYNC_PID_FILE=/tmp/rsyncd.pid
CONTROL_FILE=/tmp/control/complete
RSYNCD_CONF=/tmp/rsyncd.conf
STUNNEL_CONF=/tmp/stunnel.conf
STUNNEL_PID_FILE=/tmp/stunnel.pid
PSK_FILE=/keys/psk.txt
STUNNEL_LISTEN_PORT=8000
RSYNC_LOG=/tmp/rsyncd.log

SCRIPT_DIR="$(dirname "$(realpath "$0")")"
cd "$SCRIPT_DIR"

if [[ ! -r $PSK_FILE ]]; then
    echo "ERROR: Pre-shared key not found - $PSK_FILE"
    exit 1
fi

TARGET="/data"
BLOCK_TARGET="/dev/block"

if [[ ! -d $TARGET ]] && ! test -b $BLOCK_TARGET; then
    echo "ERROR: target location not found"
    exit 1
fi

if [[ -d $TARGET ]]; then
    ##############################
    ## Filesystem volume, use rsync

    mkdir -p "$(dirname "$CONTROL_FILE")"

    ##############################
    ## Set up rsync config

    if [[ $PRIVILEGED_MOVER -ne 0 ]]; then
        RSYNC_UID="uid = 0"
        RSYNC_GID="gid = *"
    else
        RSYNC_UID=""
        RSYNC_GID=""
    fi

    cat - > "$RSYNCD_CONF" <<RSYNCD_CONF
pid = $RSYNC_PID_FILE
address = 127.0.0.1
log file = $RSYNC_LOG
max verbosity = 10
use chroot = false
numeric ids = true
munge symlinks = false
open noatime = true
reverse lookup = false
transfer logging = true
read only = false
$RSYNC_UID
$RSYNC_GID

[data]
comment = PVC data
path = $TARGET

[control]
comment = Control files for signaling
path = $(dirname $CONTROL_FILE)
RSYNCD_CONF

    ##############################
    ## Set up stunnel config
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
; Port to listen for incoming connections from remote
accept = :::$STUNNEL_LISTEN_PORT
; We are the server
client = no
; When we get an incoming connection, spawn rsync to handle it
exec = /usr/bin/rsync
execargs = rsync --server --daemon --config=$RSYNCD_CONF .
STUNNEL_CONF

    ##############################
    ## Print version information
    rsync --version
    stunnel -version "$STUNNEL_CONF"

    ##############################
    ## Tail rsync log to stdout so it shows in pod logs
    touch "$RSYNC_LOG"
    tail -f "$RSYNC_LOG" &
    TAIL_PID="$!"

    rm -f "$CONTROL_FILE"
fi

if test -b $BLOCK_TARGET; then
    ##############################
    ## block volume, use diskrsync-tcp

    ##############################
    ## Set up stunnel config
    cat - > "$STUNNEL_CONF" <<STUNNEL_CONF
; Global options
debug = debug
foreground = no
output = /dev/stdout
pid = $STUNNEL_PID_FILE
syslog = no

[diskrsync]
ciphers = PSK
PSKsecrets = $PSK_FILE
; Port to listen for incoming connections from remote
accept = :::$STUNNEL_LISTEN_PORT
; We are the server
client = no
connect = 8888
#exec = /diskrsync-tcp
#execargs = diskrsync-tcp $BLOCK_TARGET --target --port 8888 --control-file $CONTROL_FILE
STUNNEL_CONF
fi

/diskrsync-tcp $BLOCK_TARGET --target --port 8888 --control-file $CONTROL_FILE&

##############################
## Start stunnel to wait for incoming connections
echo "Starting stunnel..."
stunnel "$STUNNEL_CONF"

##############################
## Wait for the control file to be created, signaling that we should
## terminate
echo "Waiting for control file to be created ($CONTROL_FILE)..."
while [[ ! -e $CONTROL_FILE ]]; do
    sleep 1
done

sleep 5  # Give time for the rsync connection to finish

##############################
## Terminate stunnel
echo "Shutting down..."
kill -TERM "$(<"$STUNNEL_PID_FILE")"
if [[ -d $TARGET ]]; then
    kill -TERM "$TAIL_PID"
fi
wait

sync
