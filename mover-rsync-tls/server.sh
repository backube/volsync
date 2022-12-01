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

if [[ ! -d /data ]]; then
    echo "ERROR: PVC not mounted at /data"
    exit 1
fi

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
path = /data

[control]
comment = Control files for signaling
path = $(dirname $CONTROL_FILE)
RSYNCD_CONF

##############################
## Set up stunnel config
cat - > "$STUNNEL_CONF" <<STUNNEL_CONF
; Global options
debug = info
foreground = no
output = /dev/stdout
pid = $STUNNEL_PID_FILE
syslog = no

[rsync]
ciphers = PSK
PSKsecrets = $PSK_FILE
; Level 5 is min 512b security, TLS >= 1.2, no SHA1
securityLevel = 5
sslVersionMin = TLSv1.2

; Port to listen for incoming connections
accept = :::$STUNNEL_LISTEN_PORT
; We are the server
client = no
; When we get an incoming connection, spawn rsync to handle it
exec = /usr/bin/rsync
execargs = rsync --server --daemon --config=$RSYNCD_CONF .
STUNNEL_CONF

##############################
## Tail rsync log to stdout so it shows in pod logs
touch "$RSYNC_LOG"
tail -f "$RSYNC_LOG" &
TAIL_PID="$!"

##############################
## Start stunnel to wait for incoming connections
echo "Starting stunnel..."
rm -f "$CONTROL_FILE"
stunnel "$STUNNEL_CONF"

##############################
## Wait for the control file to be created, signaling that we should
## terminate
echo "Waiting for control file to be created ($CONTROL_FILE)..."
while [[ ! -e $CONTROL_FILE ]]; do
    sleep 1
done

##############################
## Terminate stunnel
echo "Shutting down..."
kill -TERM "$(<"$STUNNEL_PID_FILE")"
kill -TERM "$TAIL_PID"
wait

sync
