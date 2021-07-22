#! /bin/bash

set -e -o pipefail

echo "Scribe rsync container version: ${version:-unknown}"

# Allow source's key to access, but restrict what it can do.
mkdir -p ~/.ssh
chmod 700 ~/.ssh
echo "command=\"/destination-command.sh\",restrict $(</keys/source.pub)" > ~/.ssh/authorized_keys

# Wait for incoming rsync transfer
echo "Waiting for connection..."
rm -f /var/run/nologin
/usr/sbin/sshd -D -e -q

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
