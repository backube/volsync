#! /bin/bash

set -e -o pipefail

echo "VolSync rsync container version: ${version:-unknown}"

# Ensure we have connection info for the destination
DESTINATION_PORT="${DESTINATION_PORT:-22}"
if [[ -z "$DESTINATION_ADDRESS" ]]; then
    echo "Remote host must be provided in DESTINATION_ADDRESS"
    exit 1
fi

mkdir -p ~/.ssh/controlmasters
chmod 711 ~/.ssh

# Provide ssh host key to validate remote
echo "$DESTINATION_ADDRESS $(</keys/destination.pub)" > ~/.ssh/known_hosts

cat - <<SSHCONFIG > ~/.ssh/config
Host *
  # Wait max 30s to establish connection
  ConnectTimeout 30
  # Control persist to speed 2nd ssh connection
  ControlMaster auto
  ControlPath ~/.ssh/controlmasters/%C
  ControlPersist 5
  # Disables warning when IP is added to known_hosts
  CheckHostIP no
  # Use the identity provided via attached Secret
  IdentityFile /keys/source
  Port ${DESTINATION_PORT}
  # Enable protocol-level keepalive to detect connection failure
  ServerAliveCountMax 4
  ServerAliveInterval 30
  # We know the key of the server, so be strict
  StrictHostKeyChecking yes
  # Using protocol-level, so we don't need TCP-level
  TCPKeepAlive no
SSHCONFIG

MAX_RETRIES=5
RETRY=0
DELAY=2
FACTOR=2
rc=1
echo "Syncing data to ${DESTINATION_ADDRESS}:${DESTINATION_PORT} ..."
START_TIME=$SECONDS
# Avoids exiting on rsync failure
set +e
while [[ ${rc} -ne 0 && ${RETRY} -lt ${MAX_RETRIES} ]]
do
    RETRY=$((RETRY + 1))
    rsync -aAhHSxz --delete --itemize-changes --info=stats2,misc2 /data/ "root@${DESTINATION_ADDRESS}":.
    rc=$?
    if [[ ${rc} -ne 0 ]]; then
        echo "Syncronization failed. Retrying in ${DELAY} seconds. Retry ${RETRY}/${MAX_RETRIES}."
        sleep ${DELAY}
        DELAY=$((DELAY * FACTOR ))
    fi
done
set -e
echo "Rsync completed in $(( SECONDS - START_TIME ))s"
sync
if [[ $rc -eq 0 ]]; then
    echo "Synchronization completed successfully. Notifying destination..."
    ssh "root@${DESTINATION_ADDRESS}" shutdown 0
else
    echo "Synchronization failed. rsync returned: $rc"
    exit $rc
fi
