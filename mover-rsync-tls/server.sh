#! /bin/bash

set -e -o pipefail

RSYNC_PID_FILE=/tmp/rsyncd.pid
CONTROL_FILE=/tmp/control/complete
RSYNCD_CONF=/tmp/rsyncd.conf
STUNNEL_CONF=/tmp/stunnel.conf
STUNNEL_PID_FILE=/tmp/stunnel.pid
PSK_FILE=/keys/psk.txt
RSYNC_LOG=/tmp/rsyncd.log
IPV6_DISABLED=$(cat /sys/module/ipv6/parameters/disable)

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
  while [[ -f "${END_DEBUG_FILE}" ]]; do
    sleep 10
  done

  echo ""
  echo "##################################################################"
  echo "Debug done, exiting."
  echo "##################################################################"
  sleep 2
  exit 0
fi

cd "$SCRIPT_DIR"

STUNNEL_LISTEN_PORT=:::8000
# If IPv6 is in disable state, the output would be "1"
if [[ $IPV6_DISABLED -eq 1 ]]; then
    STUNNEL_LISTEN_PORT=8000
fi

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
    echo "Destination PVC volumeMode is filesystem"

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
socket = l:SO_KEEPALIVE=1
socket = l:TCP_KEEPIDLE=180
socket = r:SO_KEEPALIVE=1
socket = r:TCP_KEEPIDLE=180
syslog = no

[rsync]
ciphers = PSK
PSKsecrets = $PSK_FILE
; Port to listen for incoming connections from remote
accept = $STUNNEL_LISTEN_PORT
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
    echo "Destination PVC volumeMode is block"

    ##############################
    ## Set up stunnel config
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
; Port to listen for incoming connections from remote
accept = $STUNNEL_LISTEN_PORT
; We are the server
client = no
connect = 8888
#exec = /diskrsync-tcp
#execargs = diskrsync-tcp $BLOCK_TARGET --target --port 8888 --control-file $CONTROL_FILE
STUNNEL_CONF

  /diskrsync-tcp $BLOCK_TARGET --target --port 8888 --control-file $CONTROL_FILE&
fi

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
echo "Stunnel completed shut down."

sync -f $TARGET
echo "Sync complete, exiting."
