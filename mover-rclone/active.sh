#! /bin/bash

set -e -o pipefail

echo "VolSync rclone container version: ${version:-unknown}"

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

function error {
    rc="$1"
    shift
    echo "error: $*"
    exit "$rc"
}

# Rclone config file that gets mounted as a Secret onto RCLONE_CONFIG

[[ -n "${RCLONE_DEST_PATH}" ]] || error 1 "RCLONE_DEST_PATH must be defined"
[[ -n "${DIRECTION}" ]] || error 1 "DIRECTION must be defined"
[[ -n "${PRIVILEGED_MOVER}" ]] || error 1 "PRIVILEGED_MOVER must be defined"


# Flags for the main sync operation (no --progress and no --stats-one-line-date so we can get a summary at the end)
RCLONE_FLAGS_SYNC=(--checksum --one-file-system --create-empty-src-dirs --stats 20s --transfers 10)

# Flags for the permissions.facl copy
RCLONE_FLAGS_COPY=(--checksum --one-file-system --create-empty-src-dirs --stats-one-line-date --stats 20s --transfers 10)

if [[ -n "${CUSTOM_CA}" ]]; then
    echo "Using custom CA."
    RCLONE_FLAGS_SYNC+=(--ca-cert "${CUSTOM_CA}")
    RCLONE_FLAGS_COPY+=(--ca-cert "${CUSTOM_CA}")
fi

START_TIME=$SECONDS
case "${DIRECTION}" in
source)
    getfacl -R "${MOUNT_PATH}" > /tmp/permissions.facl
    rclone sync "${RCLONE_FLAGS_SYNC[@]}" "${MOUNT_PATH}" "${RCLONE_CONFIG_SECTION}:${RCLONE_DEST_PATH}" --log-level DEBUG
    rclone copy "${RCLONE_FLAGS_COPY[@]}" --include permissions.facl /tmp "${RCLONE_CONFIG_SECTION}:${RCLONE_DEST_PATH}" --log-level DEBUG
    ;;
destination)
    rclone sync "${RCLONE_FLAGS_SYNC[@]}" --exclude permissions.facl "${RCLONE_CONFIG_SECTION}:${RCLONE_DEST_PATH}" "${MOUNT_PATH}" --log-level DEBUG
    rclone copy "${RCLONE_FLAGS_COPY[@]}" --include permissions.facl "${RCLONE_CONFIG_SECTION}:${RCLONE_DEST_PATH}" /tmp --log-level DEBUG
    stat /tmp/permissions.facl
    setfacl --restore=/tmp/permissions.facl || true
    ;;
*)
    error 1 "unknown value for DIRECTION: ${DIRECTION}"
    ;;
esac
sync
echo "Rclone completed in $(( SECONDS - START_TIME ))s"
