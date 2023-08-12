#! /bin/bash

set -e -o pipefail

echo "VolSync rsync container version: ${version:-unknown}"

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
sync
echo "Exiting... Exit code: $CODE"
exit "$CODE"
