#! /bin/bash

set -e -o pipefail

SERVER_PORT="${SERVER_PORT:-22}"
if [[ -z "$SERVER_ADDRESS" ]]; then
    echo "Remote host must be provided in SERVER_ADDRESS"
    exit 1
fi

mkdir -p ~/.ssh/controlmasters
chmod 711 ~/.ssh

# Provide ssh host key to validate remote
echo "$SERVER_ADDRESS $(</keys/server.pub)" > ~/.ssh/known_hosts

cat - <<SSHCONFIG > ~/.ssh/config
Host *
  # Control persist to speed 2nd ssh connection
  ControlMaster auto
  ControlPath ~/.ssh/controlmasters/%r@%h:%p
  ControlPersist 5
  # Disables warning when IP is added to known_hosts
  CheckHostIP no
  # Use the identity provided via attached Secret
  IdentityFile /keys/client
  Port ${SERVER_PORT}
  # Enable protocol-level keepalive to detect connection failure
  ServerAliveCountMax 4
  ServerAliveInterval 30
  # We know the key of the server, so be strict
  StrictHostKeyChecking yes
  # Using protocol-level, so we don't need TCP-level
  TCPKeepAlive no
SSHCONFIG

echo "Syncing data to ${SERVER_ADDRESS}:${SERVER_PORT} ..."
rsync -aAhHSxXz --delete --itemize-changes --info=stats2,misc2 /data/ "root@${SERVER_ADDRESS}":.
rc=$?
if [[ $rc -eq 0 ]]; then
    echo "Synchronization completed successfully. Calling shutdown."
    ssh "root@${SERVER_ADDRESS}" shutdown 0
else
    echo "Synchronization failed. rsync returned: $rc"
    exit $rc
fi
