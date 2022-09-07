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

RCLONE_FLAGS=(--checksum --one-file-system --create-empty-src-dirs --progress --stats-one-line-date --stats 20s --transfers 10)

START_TIME=$SECONDS
case "${DIRECTION}" in
source)
    getfacl -R "${MOUNT_PATH}" > /tmp/permissions.facl
    rclone sync "${RCLONE_FLAGS[@]}" "${MOUNT_PATH}" "${RCLONE_CONFIG_SECTION}:${RCLONE_DEST_PATH}" --log-level DEBUG
    rclone copy "${RCLONE_FLAGS[@]}" --include permissions.facl /tmp "${RCLONE_CONFIG_SECTION}:${RCLONE_DEST_PATH}" --log-level DEBUG
    ;;
destination)
    rclone sync "${RCLONE_FLAGS[@]}" --exclude permissions.facl "${RCLONE_CONFIG_SECTION}:${RCLONE_DEST_PATH}" "${MOUNT_PATH}" --log-level DEBUG
    rclone copy "${RCLONE_FLAGS[@]}" --include permissions.facl "${RCLONE_CONFIG_SECTION}:${RCLONE_DEST_PATH}" /tmp --log-level DEBUG
    stat /tmp/permissions.facl
    setfacl --restore=/tmp/permissions.facl || true
    ;;
*)
    error 1 "unknown value for DIRECTION: ${DIRECTION}"
    ;;
esac
sync
echo "Rclone completed in $(( SECONDS - START_TIME ))s"
