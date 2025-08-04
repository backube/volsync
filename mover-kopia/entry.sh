#! /bin/bash

echo "Starting container"

set -e -o pipefail

echo "VolSync kopia container version: ${version:-unknown}"
echo  "$@"

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

declare -a KOPIA
KOPIA=("kopia")
if [[ -n "${CUSTOM_CA}" ]]; then
    echo "Using custom CA."
    export KOPIA_CA_CERT="${CUSTOM_CA}"
fi

"${KOPIA[@]}" --version

# Print an error message and exit
# error rc "message"
function error {
    echo "ERROR: $2"
    exit "$1"
}

# Error and exit if a variable isn't defined
# check_var_defined "MY_VAR"
function check_var_defined {
    if [[ -z ${!1} ]]; then
        error 1 "$1 must be defined"
    fi
}

# Execute pre/post snapshot actions following Kopia's native approach
# execute_action "action_command" "action_type"
function execute_action {
    local action_command="$1"
    local action_type="$2"
    
    if [[ -z "${action_command}" ]]; then
        return 0
    fi
    
    echo "Running ${action_type} action: ${action_command}"
    
    # Execute the command following Kopia's native approach
    # Kopia natively supports actions without command restrictions
    # Security is handled through:
    # 1. Container security context (non-root, limited capabilities)
    # 2. User responsibility for action content  
    # 3. Actions must be explicitly configured by users
    # 4. Container isolation limits potential damage
    
    # Execute with timeout for safety and proper error handling
    if ! timeout 300 bash -c "${action_command}"; then
        echo "ERROR: ${action_type} action failed or timed out after 300 seconds"
        return 1
    fi
    
    echo "${action_type} action completed successfully"
    return 0
}

function check_contents {
    echo "== Checking directory for content ==="
    DIR_CONTENTS="$(ls -A "${DATA_DIR}" --ignore="lost+found")"
    if [ -z "${DIR_CONTENTS}" ]; then
        echo "== Directory is empty skipping backup ==="
        exit 0
    fi
}

# Connect to or create the repository
function ensure_connected {
    echo "=== Connecting to repository ==="
    set +e  # Don't exit on command failure
    
    # Try to connect to existing repository
    outfile=$(mktemp -q)
    timeout 10s "${KOPIA[@]}" repository status > /dev/null 2>"$outfile"
    rc=$?
    
    if [[ $rc -ne 0 ]]; then
        echo "Repository not connected, attempting to connect or create..."
        # Try to connect first
        "${KOPIA[@]}" repository connect from-config --config-file /credentials/repository.config 2>"$outfile" || {
            # If connect fails, try to create a new repository
            echo "Creating new repository..."
            create_repository
        }
    else
        echo "Repository already connected"
    fi
    
    set -e
    rm -f "$outfile"
}

function create_repository {
    echo "=== Creating repository ==="
    
    # Determine repository type from environment variables
    if [[ -n "${KOPIA_S3_BUCKET}" ]]; then
        echo "Creating S3 repository"
        "${KOPIA[@]}" repository create s3 \
            --bucket="${KOPIA_S3_BUCKET}" \
            --endpoint="${KOPIA_S3_ENDPOINT:-s3.amazonaws.com}" \
            --access-key="${AWS_ACCESS_KEY_ID}" \
            --secret-access-key="${AWS_SECRET_ACCESS_KEY}"
    elif [[ -n "${KOPIA_AZURE_CONTAINER}" ]]; then
        echo "Creating Azure repository"
        "${KOPIA[@]}" repository create azure \
            --container="${KOPIA_AZURE_CONTAINER}" \
            --storage-account="${KOPIA_AZURE_STORAGE_ACCOUNT}" \
            --storage-key="${KOPIA_AZURE_STORAGE_KEY}"
    elif [[ -n "${KOPIA_GCS_BUCKET}" ]]; then
        echo "Creating GCS repository"
        "${KOPIA[@]}" repository create gcs \
            --bucket="${KOPIA_GCS_BUCKET}" \
            --credentials-file="${GOOGLE_APPLICATION_CREDENTIALS}"
    elif [[ -n "${KOPIA_FS_PATH}" ]]; then
        echo "Creating filesystem repository"
        "${KOPIA[@]}" repository create filesystem --path="${KOPIA_FS_PATH}"
    else
        error 1 "No repository configuration found"
    fi
}

function do_backup {
    echo "=== Starting backup ==="
    
    # Build snapshot command with options
    declare -a SNAPSHOT_CMD
    SNAPSHOT_CMD=("${KOPIA[@]}" "snapshot" "create" "${DATA_DIR}")
    
    # Add compression if specified
    if [[ -n "${KOPIA_COMPRESSION}" ]]; then
        SNAPSHOT_CMD+=(--compression="${KOPIA_COMPRESSION}")
    fi
    
    # Add parallelism if specified
    if [[ -n "${KOPIA_PARALLELISM}" ]]; then
        SNAPSHOT_CMD+=(--parallel="${KOPIA_PARALLELISM}")
    fi
    
    # Run before-snapshot action if specified
    if [[ -n "${KOPIA_BEFORE_SNAPSHOT}" ]]; then
        if ! execute_action "${KOPIA_BEFORE_SNAPSHOT}" "before-snapshot"; then
            echo "ERROR: Before-snapshot action failed"
            exit 1
        fi
    fi
    
    # Create snapshot
    "${SNAPSHOT_CMD[@]}"
    
    # Run after-snapshot action if specified
    if [[ -n "${KOPIA_AFTER_SNAPSHOT}" ]]; then
        if ! execute_action "${KOPIA_AFTER_SNAPSHOT}" "after-snapshot"; then
            echo "WARNING: After-snapshot action failed, but snapshot was created successfully"
            # Don't exit here since the snapshot was already created
        fi
    fi
}

function do_maintenance {
    echo "=== Starting maintenance ==="
    "${KOPIA[@]}" maintenance run --full
}

function do_retention {
    echo "=== Applying retention policy ==="
    
    declare -a POLICY_CMD
    POLICY_CMD=("${KOPIA[@]}" "policy" "set" "${DATA_DIR}")
    
    # Build retention policy options
    if [[ -n "${KOPIA_RETAIN_HOURLY}" ]]; then
        POLICY_CMD+=(--keep-hourly="${KOPIA_RETAIN_HOURLY}")
    fi
    if [[ -n "${KOPIA_RETAIN_DAILY}" ]]; then
        POLICY_CMD+=(--keep-daily="${KOPIA_RETAIN_DAILY}")
    fi
    if [[ -n "${KOPIA_RETAIN_WEEKLY}" ]]; then
        POLICY_CMD+=(--keep-weekly="${KOPIA_RETAIN_WEEKLY}")
    fi
    if [[ -n "${KOPIA_RETAIN_MONTHLY}" ]]; then
        POLICY_CMD+=(--keep-monthly="${KOPIA_RETAIN_MONTHLY}")
    fi
    if [[ -n "${KOPIA_RETAIN_YEARLY}" ]]; then
        POLICY_CMD+=(--keep-annual="${KOPIA_RETAIN_YEARLY}")
    fi
    
    # Apply policy if any retention options are set
    if [[ ${#POLICY_CMD[@]} -gt 4 ]]; then
        "${POLICY_CMD[@]}"
    fi
}

function select_snapshot_to_restore {
    echo "Selecting snapshot to restore"
    
    # List snapshots and find the appropriate one
    if [[ -n "${KOPIA_RESTORE_AS_OF}" ]]; then
        echo "Restoring as of: ${KOPIA_RESTORE_AS_OF}"
        "${KOPIA[@]}" snapshot list --json | jq -r ".[] | select(.startTime <= \"${KOPIA_RESTORE_AS_OF}\") | .id" | head -1
    elif [[ -n "${KOPIA_SHALLOW}" ]]; then
        echo "Shallow restore, showing last ${KOPIA_SHALLOW} snapshots"
        "${KOPIA[@]}" snapshot list --json | jq -r ".[0:${KOPIA_SHALLOW}][] | .id" | head -1
    else
        # Get latest snapshot
        "${KOPIA[@]}" snapshot list --json | jq -r ".[0].id"
    fi
}

function do_restore {
    echo "=== Starting restore ==="
    
    # Select snapshot to restore
    local snapshot_id
    snapshot_id=$(select_snapshot_to_restore)
    
    if [[ -z ${snapshot_id} ]]; then
        echo "No eligible snapshots found"
        echo "=== No data will be restored ==="
        return
    fi
    
    echo "Selected snapshot with id: ${snapshot_id}"
    
    # Restore the snapshot
    pushd "${DATA_DIR}"
    "${KOPIA[@]}" snapshot restore "${snapshot_id}" .
    popd
}

echo "Testing mandatory env variables"
# Check the mandatory env variables
for var in KOPIA_PASSWORD \
           DATA_DIR \
           ; do
    check_var_defined $var
done

# Set cache directory if specified
if [[ -n "${CACHE_DIR}" ]]; then
    export KOPIA_CACHE_DIRECTORY="${CACHE_DIR}"
fi

# Set password
export KOPIA_PASSWORD

START_TIME=$SECONDS

# Determine operation based on DIRECTION or arguments
if [[ "${DIRECTION}" == "source" ]]; then
    echo "=== Running as SOURCE ==="
    check_contents
    ensure_connected
    do_backup
    do_retention
    do_maintenance
elif [[ "${DIRECTION}" == "destination" ]]; then
    echo "=== Running as DESTINATION ==="
    ensure_connected
    do_restore
    sync -f "${DATA_DIR}"
else
    # Legacy mode - execute operations based on arguments
    for op in "$@"; do
        case $op in
            "backup")
                check_contents
                ensure_connected
                do_backup
                do_retention
                ;;
            "restore")
                ensure_connected
                do_restore
                sync -f "${DATA_DIR}"
                ;;
            "maintenance")
                ensure_connected
                do_maintenance
                ;;
            *)
                error 2 "unknown operation: $op"
                ;;
        esac
    done
fi

echo "Kopia completed in $(( SECONDS - START_TIME ))s"
echo "=== Done ==="