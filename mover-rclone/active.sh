#! /bin/bash

set -e -o pipefail

echo "VolSync rclone container version: ${version:-unknown}"

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
