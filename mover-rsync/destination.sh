#! /bin/bash

set -e -o pipefail

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

# Allow source's key to access, but restrict what it can do.
mkdir -p ~/.ssh
chmod 700 ~/.ssh
echo "command=\"/mover-rsync/destination-command.sh\",restrict $(</keys/source.pub)" > ~/.ssh/authorized_keys

VOLUME_MODE="filesystem"
if test -b /dev/block; then
    VOLUME_MODE=block
fi
echo "Destination PVC volumeMode is $VOLUME_MODE"

# Wait for incoming rsync transfer
echo "Waiting for connection..."
rm -f /var/run/nologin
/usr/sbin/sshd -D -e -p 8022

# When sshd exits, need to return the proper exit code from the rsync operation
CODE=255
if [[ -e /tmp/exit_code ]]; then
    CODE_IN="$(</tmp/exit_code)"
    if [[ $CODE_IN =~ ^[0-9]+$ ]]; then
        CODE="$CODE_IN"
    fi
fi
sync -f $HOME
echo "Exiting... Exit code: $CODE"
exit "$CODE"
