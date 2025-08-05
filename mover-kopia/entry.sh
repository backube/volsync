#! /bin/bash

echo "Starting container"
echo

set -e -o pipefail

echo "VolSync kopia container version: ${version:-unknown}"
echo "$@"
echo

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
# Set both cache and log directories to writable mounted location
export KOPIA_CACHE_DIRECTORY="${KOPIA_CACHE_DIR}"
export KOPIA_LOG_DIR="${KOPIA_CACHE_DIR}/logs"

# Create necessary directories upfront
mkdir -p "${KOPIA_CACHE_DIR}/logs"
chmod 755 "${KOPIA_CACHE_DIR}/logs"

# DEBUG: Show environment and directory setup
echo "=== DEBUG: Environment Setup ==="
echo "HOME: ${HOME}"
echo "KOPIA_CACHE_DIR: ${KOPIA_CACHE_DIR}"
echo "KOPIA_CACHE_DIRECTORY: ${KOPIA_CACHE_DIRECTORY}"
echo "KOPIA_LOG_DIR: ${KOPIA_LOG_DIR}"
echo "Current user: $(whoami)"
echo "Current working directory: $(pwd)"
echo "Cache directory writable: $(test -w ${KOPIA_CACHE_DIR} && echo 'Yes' || echo 'No')"
echo "Log directory exists: $(test -d ${KOPIA_LOG_DIR} && echo 'Yes' || echo 'No')"
echo "Contents of cache directory: $(ls -la ${KOPIA_CACHE_DIR} 2>/dev/null || echo 'Directory does not exist')"
echo ""
echo "=== ENVIRONMENT VARIABLES STATUS ==="
echo "KOPIA_REPOSITORY: ${KOPIA_REPOSITORY:+[SET]}${KOPIA_REPOSITORY:-[NOT SET]}"
echo "KOPIA_PASSWORD: ${KOPIA_PASSWORD:+[SET]}${KOPIA_PASSWORD:-[NOT SET]}"
echo "KOPIA_S3_BUCKET: ${KOPIA_S3_BUCKET:+[SET]}${KOPIA_S3_BUCKET:-[NOT SET]}"
echo "KOPIA_S3_ENDPOINT: ${KOPIA_S3_ENDPOINT:+[SET]}${KOPIA_S3_ENDPOINT:-[NOT SET]}"
echo "KOPIA_S3_DISABLE_TLS: ${KOPIA_S3_DISABLE_TLS:+[SET]}${KOPIA_S3_DISABLE_TLS:-[NOT SET]}"
echo "AWS_ACCESS_KEY_ID: ${AWS_ACCESS_KEY_ID:+[SET]}${AWS_ACCESS_KEY_ID:-[NOT SET]}"
echo "AWS_SECRET_ACCESS_KEY: ${AWS_SECRET_ACCESS_KEY:+[SET]}${AWS_SECRET_ACCESS_KEY:-[NOT SET]}"
echo "AWS_DEFAULT_REGION: ${AWS_DEFAULT_REGION:+[SET]}${AWS_DEFAULT_REGION:-[NOT SET]}"
echo "KOPIA_OVERRIDE_USERNAME: ${KOPIA_OVERRIDE_USERNAME:+[SET]}${KOPIA_OVERRIDE_USERNAME:-[NOT SET]}"
echo "KOPIA_OVERRIDE_HOSTNAME: ${KOPIA_OVERRIDE_HOSTNAME:+[SET]}${KOPIA_OVERRIDE_HOSTNAME:-[NOT SET]}"
echo "KOPIA_SOURCE_PATH_OVERRIDE: ${KOPIA_SOURCE_PATH_OVERRIDE:+[SET]}${KOPIA_SOURCE_PATH_OVERRIDE:-[NOT SET]}"
echo ""
echo "=== Additional Backend Environment Variables ==="
echo "KOPIA_B2_BUCKET: ${KOPIA_B2_BUCKET:+[SET]}${KOPIA_B2_BUCKET:-[NOT SET]}"
echo "B2_ACCOUNT_ID: ${B2_ACCOUNT_ID:+[SET]}${B2_ACCOUNT_ID:-[NOT SET]}"
echo "B2_APPLICATION_KEY: ${B2_APPLICATION_KEY:+[SET]}${B2_APPLICATION_KEY:-[NOT SET]}"
echo "WEBDAV_URL: ${WEBDAV_URL:+[SET]}${WEBDAV_URL:-[NOT SET]}"
echo "WEBDAV_USERNAME: ${WEBDAV_USERNAME:+[SET]}${WEBDAV_USERNAME:-[NOT SET]}"
echo "WEBDAV_PASSWORD: ${WEBDAV_PASSWORD:+[SET]}${WEBDAV_PASSWORD:-[NOT SET]}"
echo "SFTP_HOST: ${SFTP_HOST:+[SET]}${SFTP_HOST:-[NOT SET]}"
echo "SFTP_PORT: ${SFTP_PORT:+[SET]}${SFTP_PORT:-[NOT SET]}"
echo "SFTP_USERNAME: ${SFTP_USERNAME:+[SET]}${SFTP_USERNAME:-[NOT SET]}"
echo "SFTP_PASSWORD: ${SFTP_PASSWORD:+[SET]}${SFTP_PASSWORD:-[NOT SET]}"
echo "SFTP_PATH: ${SFTP_PATH:+[SET]}${SFTP_PATH:-[NOT SET]}"
echo "SFTP_KEY_FILE: ${SFTP_KEY_FILE:+[SET]}${SFTP_KEY_FILE:-[NOT SET]}"
echo "RCLONE_REMOTE_PATH: ${RCLONE_REMOTE_PATH:+[SET]}${RCLONE_REMOTE_PATH:-[NOT SET]}"
echo "RCLONE_EXE: ${RCLONE_EXE:+[SET]}${RCLONE_EXE:-[NOT SET]}"
echo "RCLONE_CONFIG: ${RCLONE_CONFIG:+[SET]}${RCLONE_CONFIG:-[NOT SET]}"
echo "GOOGLE_DRIVE_FOLDER_ID: ${GOOGLE_DRIVE_FOLDER_ID:+[SET]}${GOOGLE_DRIVE_FOLDER_ID:-[NOT SET]}"
echo "GOOGLE_DRIVE_CREDENTIALS: ${GOOGLE_DRIVE_CREDENTIALS:+[SET]}${GOOGLE_DRIVE_CREDENTIALS:-[NOT SET]}"
echo "=== END DEBUG ==="
echo ""

KOPIA=("kopia" "--config-file=${KOPIA_CACHE_DIR}/kopia.config" "--log-dir=${KOPIA_CACHE_DIR}/logs")
if [[ -n "${CUSTOM_CA}" ]]; then
    echo "Using custom CA."
    export KOPIA_CA_CERT="${CUSTOM_CA}"
fi

echo "=== Kopia Version ==="
"${KOPIA[@]}" --version
echo "====================="

# Print an error message and exit
# error rc "message"
function error {
    echo ""  # Add blank line before error
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

# Add username/hostname overrides to command array if specified
# add_user_overrides command_array_name
function add_user_overrides {
    local -n cmd_array=$1
    
    if [[ -n "${KOPIA_OVERRIDE_USERNAME}" ]]; then
        echo "Using username override: ${KOPIA_OVERRIDE_USERNAME}"
        cmd_array+=(--override-username="${KOPIA_OVERRIDE_USERNAME}")
    fi
    if [[ -n "${KOPIA_OVERRIDE_HOSTNAME}" ]]; then
        echo "Using hostname override: ${KOPIA_OVERRIDE_HOSTNAME}"
        cmd_array+=(--override-hostname="${KOPIA_OVERRIDE_HOSTNAME}")
    fi
}

# Apply policy configuration if available
function apply_policy_config {
    if [[ -n "${KOPIA_CONFIG_PATH}" && -d "${KOPIA_CONFIG_PATH}" ]]; then
        echo "=== Applying policy configuration ==="
        
        # Apply global policy if available
        if [[ -n "${KOPIA_GLOBAL_POLICY_FILE}" && -f "${KOPIA_GLOBAL_POLICY_FILE}" ]]; then
            echo "Importing global policy from ${KOPIA_GLOBAL_POLICY_FILE}"
            if "${KOPIA[@]}" policy import --from-file "${KOPIA_GLOBAL_POLICY_FILE}" --delete-other-policies "(global)"; then
                echo "Global policy imported successfully"
            else
                echo "Warning: Failed to import global policy, continuing with default policies"
            fi
        fi
        
        # Apply repository configuration if available  
        if [[ -n "${KOPIA_REPOSITORY_CONFIG_FILE}" && -f "${KOPIA_REPOSITORY_CONFIG_FILE}" ]]; then
            echo "Applying repository configuration from ${KOPIA_REPOSITORY_CONFIG_FILE}"
            # Repository configuration typically includes settings like enableActions
            # This would need to be parsed and applied appropriately
            echo "Note: Repository configuration parsing not yet implemented, file found at ${KOPIA_REPOSITORY_CONFIG_FILE}"
        fi
    fi
}

# Connect to or create the repository
function ensure_connected {
    echo "=== Connecting to repository ==="
    
    # Try to connect to existing repository (let errors display naturally)
    if ! timeout 10s "${KOPIA[@]}" repository status >/dev/null 2>&1; then
        echo "Repository not connected, attempting to connect or create..."
        echo ""
        
        # Disable exit on error for connection attempts
        set +e
        
        # Try to connect first (if config exists)
        if [[ -f /credentials/repository.config ]]; then
            echo "Attempting to connect from config file..."
            "${KOPIA[@]}" repository connect from-config --config-file /credentials/repository.config
            local config_result=$?
            
            if [[ $config_result -ne 0 ]]; then
                echo "Config connection failed, trying direct connection..."
                echo ""
                connect_repository
                local direct_result=$?
                
                if [[ $direct_result -ne 0 ]]; then
                    echo "Direct connection failed, creating new repository..."
                    echo ""
                    create_repository
                    local create_result=$?
                    if [[ $create_result -ne 0 ]]; then
                        set -e  # Re-enable exit on error
                        error 1 "Failed to create repository"
                    fi
                fi
            fi
        else
            # No config file, try direct connection
            echo "Attempting to connect to existing repository..."
            connect_repository
            local direct_result=$?
            
            if [[ $direct_result -ne 0 ]]; then
                echo "Connection failed, creating new repository..."
                echo ""
                create_repository
                local create_result=$?
                if [[ $create_result -ne 0 ]]; then
                    set -e  # Re-enable exit on error
                    error 1 "Failed to create repository"
                fi
            fi
        fi
        
        # Re-enable exit on error
        set -e
    else
        echo "Repository already connected"
    fi
    
    echo ""
    
    # Set cache directory after successful connection
    echo "=== Setting cache directory ==="
    if ! "${KOPIA[@]}" cache set --cache-directory="${KOPIA_CACHE_DIR}"; then
        error 1 "Failed to set cache directory"
    fi
    echo "Cache directory configured successfully"
    echo ""
    
    # Apply policy configuration after connection
    apply_policy_config
}

function connect_repository {
    echo "=== Connecting to existing repository ==="
    
    # Determine repository type from environment variables and connect
    if [[ -n "${KOPIA_S3_BUCKET}" ]]; then
        echo "Connecting to S3 repository"
        echo ""
        echo "=== S3 Connection Debug ==="
        echo "KOPIA_S3_BUCKET: ${KOPIA_S3_BUCKET:+[SET]}${KOPIA_S3_BUCKET:-[NOT SET]}"
        echo "KOPIA_S3_ENDPOINT: ${KOPIA_S3_ENDPOINT:+[SET]}${KOPIA_S3_ENDPOINT:-[NOT SET]}"
        echo "AWS_ACCESS_KEY_ID: ${AWS_ACCESS_KEY_ID:+[SET]}${AWS_ACCESS_KEY_ID:-[NOT SET]}"
        echo "AWS_SECRET_ACCESS_KEY: ${AWS_SECRET_ACCESS_KEY:+[SET]}${AWS_SECRET_ACCESS_KEY:-[NOT SET]}"
        echo "KOPIA_REPOSITORY: ${KOPIA_REPOSITORY:+[SET]}${KOPIA_REPOSITORY:-[NOT SET]}"
        echo "KOPIA_S3_DISABLE_TLS: ${KOPIA_S3_DISABLE_TLS:+[SET]}${KOPIA_S3_DISABLE_TLS:-[NOT SET]}"
        echo "KOPIA_PASSWORD: ${KOPIA_PASSWORD:+[SET]}${KOPIA_PASSWORD:-[NOT SET]}"
        
        S3_CONNECT_CMD=("${KOPIA[@]}" repository connect s3 \
            --bucket="${KOPIA_S3_BUCKET}" \
            --endpoint="${KOPIA_S3_ENDPOINT:-s3.amazonaws.com}" \
            --access-key="${AWS_ACCESS_KEY_ID}" \
            --secret-access-key="${AWS_SECRET_ACCESS_KEY}")
        
        # Extract prefix from KOPIA_REPOSITORY (e.g., s3://bucket/prefix -> prefix)
        if [[ "${KOPIA_REPOSITORY}" =~ s3://[^/]+/(.+) ]]; then
            S3_PREFIX="${BASH_REMATCH[1]}"
            # Validate S3 prefix for security
            if [[ "${S3_PREFIX}" =~ ^[a-zA-Z0-9._/-]+$ ]] && [[ ! "${S3_PREFIX}" =~ \.\. ]]; then
                echo "Using S3 prefix: ${S3_PREFIX}"
                S3_CONNECT_CMD+=(--prefix="${S3_PREFIX}")
            else
                echo "ERROR: Invalid S3 prefix format. Only alphanumeric, dots, dashes, underscores and forward slashes allowed"
                return 1
            fi
        else
            echo "No S3 prefix detected in KOPIA_REPOSITORY"
        fi
        
        # Add disable TLS flag if specified
        if [[ "${KOPIA_S3_DISABLE_TLS}" == "true" ]]; then
            S3_CONNECT_CMD+=(--disable-tls)
        fi
        
        # Add username/hostname overrides if specified
        add_user_overrides S3_CONNECT_CMD
        
        echo "=== End S3 Connection Debug ==="
        echo ""
        echo "Executing connection command..."
        "${S3_CONNECT_CMD[@]}"
    elif [[ -n "${KOPIA_AZURE_CONTAINER}" ]]; then
        echo "Connecting to Azure repository"
        AZURE_CONNECT_CMD=("${KOPIA[@]}" repository connect azure \
            --container="${KOPIA_AZURE_CONTAINER}" \
            --storage-account="${KOPIA_AZURE_STORAGE_ACCOUNT}" \
            --storage-key="${KOPIA_AZURE_STORAGE_KEY}")
        add_user_overrides AZURE_CONNECT_CMD
        "${AZURE_CONNECT_CMD[@]}"
    elif [[ -n "${KOPIA_GCS_BUCKET}" ]]; then
        echo "Connecting to GCS repository"
        GCS_CONNECT_CMD=("${KOPIA[@]}" repository connect gcs \
            --bucket="${KOPIA_GCS_BUCKET}" \
            --credentials-file="${GOOGLE_APPLICATION_CREDENTIALS}")
        add_user_overrides GCS_CONNECT_CMD
        "${GCS_CONNECT_CMD[@]}"
    elif [[ -n "${KOPIA_FS_PATH}" ]]; then
        echo "Connecting to filesystem repository"
        FS_CONNECT_CMD=("${KOPIA[@]}" repository connect filesystem --path="${KOPIA_FS_PATH}")
        add_user_overrides FS_CONNECT_CMD
        "${FS_CONNECT_CMD[@]}"
    elif [[ -n "${KOPIA_B2_BUCKET}" ]]; then
        echo "Connecting to Backblaze B2 repository"
        B2_CONNECT_CMD=("${KOPIA[@]}" repository connect b2 \
            --bucket="${KOPIA_B2_BUCKET}" \
            --key-id="${B2_ACCOUNT_ID}" \
            --key="${B2_APPLICATION_KEY}")
        add_user_overrides B2_CONNECT_CMD
        "${B2_CONNECT_CMD[@]}"
    elif [[ -n "${WEBDAV_URL}" ]]; then
        echo "Connecting to WebDAV repository"
        WEBDAV_CONNECT_CMD=("${KOPIA[@]}" repository connect webdav \
            --url="${WEBDAV_URL}" \
            --username="${WEBDAV_USERNAME}" \
            --password="${WEBDAV_PASSWORD}")
        add_user_overrides WEBDAV_CONNECT_CMD
        "${WEBDAV_CONNECT_CMD[@]}"
    elif [[ -n "${SFTP_HOST}" ]]; then
        echo "Connecting to SFTP repository"
        SFTP_CONNECT_CMD=("${KOPIA[@]}" repository connect sftp \
            --host="${SFTP_HOST}" \
            --username="${SFTP_USERNAME}" \
            --path="${SFTP_PATH}")
        if [[ -n "${SFTP_PORT}" ]]; then
            SFTP_CONNECT_CMD+=(--port="${SFTP_PORT}")
        fi
        if [[ -n "${SFTP_PASSWORD}" ]]; then
            SFTP_CONNECT_CMD+=(--password="${SFTP_PASSWORD}")
        fi
        if [[ -n "${SFTP_KEY_FILE}" ]]; then
            SFTP_CONNECT_CMD+=(--keyfile="${SFTP_KEY_FILE}")
        fi
        add_user_overrides SFTP_CONNECT_CMD
        "${SFTP_CONNECT_CMD[@]}"
    elif [[ -n "${RCLONE_REMOTE_PATH}" ]]; then
        echo "Connecting to Rclone repository"
        RCLONE_CONNECT_CMD=("${KOPIA[@]}" repository connect rclone \
            --remote-path="${RCLONE_REMOTE_PATH}")
        if [[ -n "${RCLONE_EXE}" ]]; then
            RCLONE_CONNECT_CMD+=(--rclone-exe="${RCLONE_EXE}")
        fi
        if [[ -n "${RCLONE_CONFIG}" ]]; then
            RCLONE_CONNECT_CMD+=(--rclone-config="${RCLONE_CONFIG}")
        fi
        add_user_overrides RCLONE_CONNECT_CMD
        "${RCLONE_CONNECT_CMD[@]}"
    elif [[ -n "${GOOGLE_DRIVE_FOLDER_ID}" ]]; then
        echo "Connecting to Google Drive repository"
        GDRIVE_CONNECT_CMD=("${KOPIA[@]}" repository connect gdrive \
            --folder-id="${GOOGLE_DRIVE_FOLDER_ID}" \
            --credentials-file="${GOOGLE_DRIVE_CREDENTIALS}")
        add_user_overrides GDRIVE_CONNECT_CMD
        "${GDRIVE_CONNECT_CMD[@]}"
    else
        echo "No repository configuration found for connecting"
        return 1
    fi
}

function create_repository {
    echo "=== Creating repository ==="
    
    # Determine repository type from environment variables
    if [[ -n "${KOPIA_S3_BUCKET}" ]]; then
        echo "Creating S3 repository"
        echo ""
        echo "=== S3 Creation Debug ==="
        echo "KOPIA_S3_BUCKET: ${KOPIA_S3_BUCKET:+[SET]}${KOPIA_S3_BUCKET:-[NOT SET]}"
        echo "KOPIA_S3_ENDPOINT: ${KOPIA_S3_ENDPOINT:+[SET]}${KOPIA_S3_ENDPOINT:-[NOT SET]}"
        echo "AWS_ACCESS_KEY_ID: ${AWS_ACCESS_KEY_ID:+[SET]}${AWS_ACCESS_KEY_ID:-[NOT SET]}"
        echo "AWS_SECRET_ACCESS_KEY: ${AWS_SECRET_ACCESS_KEY:+[SET]}${AWS_SECRET_ACCESS_KEY:-[NOT SET]}"
        echo "KOPIA_REPOSITORY: ${KOPIA_REPOSITORY:+[SET]}${KOPIA_REPOSITORY:-[NOT SET]}"
        echo "KOPIA_S3_DISABLE_TLS: ${KOPIA_S3_DISABLE_TLS:+[SET]}${KOPIA_S3_DISABLE_TLS:-[NOT SET]}"
        echo "KOPIA_PASSWORD: ${KOPIA_PASSWORD:+[SET]}${KOPIA_PASSWORD:-[NOT SET]}"
        echo "KOPIA_CACHE_DIR: ${KOPIA_CACHE_DIR:+[SET]}${KOPIA_CACHE_DIR:-[NOT SET]}"
        
        S3_CREATE_CMD=("${KOPIA[@]}" repository create s3 \
            --bucket="${KOPIA_S3_BUCKET}" \
            --endpoint="${KOPIA_S3_ENDPOINT:-s3.amazonaws.com}" \
            --access-key="${AWS_ACCESS_KEY_ID}" \
            --secret-access-key="${AWS_SECRET_ACCESS_KEY}" \
            --cache-directory="${KOPIA_CACHE_DIR}")
        
        # Extract prefix from KOPIA_REPOSITORY (e.g., s3://bucket/prefix -> prefix)
        if [[ "${KOPIA_REPOSITORY}" =~ s3://[^/]+/(.+) ]]; then
            S3_PREFIX="${BASH_REMATCH[1]}"
            # Validate S3 prefix for security
            if [[ "${S3_PREFIX}" =~ ^[a-zA-Z0-9._/-]+$ ]] && [[ ! "${S3_PREFIX}" =~ \.\. ]]; then
                echo "Using S3 prefix: ${S3_PREFIX}"
                S3_CREATE_CMD+=(--prefix="${S3_PREFIX}")
            else
                echo "ERROR: Invalid S3 prefix format. Only alphanumeric, dots, dashes, underscores and forward slashes allowed"
                return 1
            fi
        else
            echo "No S3 prefix detected in KOPIA_REPOSITORY"
        fi
        
        # Add disable TLS flag if specified
        if [[ "${KOPIA_S3_DISABLE_TLS}" == "true" ]]; then
            S3_CREATE_CMD+=(--disable-tls)
        fi
        
        # Add username/hostname overrides if specified
        add_user_overrides S3_CREATE_CMD
        
        echo "=== End S3 Creation Debug ==="
        echo ""
        echo "Executing creation command..."
        "${S3_CREATE_CMD[@]}"
    elif [[ -n "${KOPIA_AZURE_CONTAINER}" ]]; then
        echo "Creating Azure repository"
        AZURE_CREATE_CMD=("${KOPIA[@]}" repository create azure \
            --container="${KOPIA_AZURE_CONTAINER}" \
            --storage-account="${KOPIA_AZURE_STORAGE_ACCOUNT}" \
            --storage-key="${KOPIA_AZURE_STORAGE_KEY}")
        add_user_overrides AZURE_CREATE_CMD
        "${AZURE_CREATE_CMD[@]}"
    elif [[ -n "${KOPIA_GCS_BUCKET}" ]]; then
        echo "Creating GCS repository"
        GCS_CREATE_CMD=("${KOPIA[@]}" repository create gcs \
            --bucket="${KOPIA_GCS_BUCKET}" \
            --credentials-file="${GOOGLE_APPLICATION_CREDENTIALS}")
        add_user_overrides GCS_CREATE_CMD
        "${GCS_CREATE_CMD[@]}"
    elif [[ -n "${KOPIA_FS_PATH}" ]]; then
        echo "Creating filesystem repository"
        FS_CREATE_CMD=("${KOPIA[@]}" repository create filesystem --path="${KOPIA_FS_PATH}")
        add_user_overrides FS_CREATE_CMD
        "${FS_CREATE_CMD[@]}"
    elif [[ -n "${KOPIA_B2_BUCKET}" ]]; then
        echo "Creating Backblaze B2 repository"
        B2_CREATE_CMD=("${KOPIA[@]}" repository create b2 \
            --bucket="${KOPIA_B2_BUCKET}" \
            --key-id="${B2_ACCOUNT_ID}" \
            --key="${B2_APPLICATION_KEY}")
        add_user_overrides B2_CREATE_CMD
        "${B2_CREATE_CMD[@]}"
    elif [[ -n "${WEBDAV_URL}" ]]; then
        echo "Creating WebDAV repository"
        WEBDAV_CREATE_CMD=("${KOPIA[@]}" repository create webdav \
            --url="${WEBDAV_URL}" \
            --username="${WEBDAV_USERNAME}" \
            --password="${WEBDAV_PASSWORD}")
        add_user_overrides WEBDAV_CREATE_CMD
        "${WEBDAV_CREATE_CMD[@]}"
    elif [[ -n "${SFTP_HOST}" ]]; then
        echo "Creating SFTP repository"
        SFTP_CREATE_CMD=("${KOPIA[@]}" repository create sftp \
            --host="${SFTP_HOST}" \
            --username="${SFTP_USERNAME}" \
            --path="${SFTP_PATH}")
        if [[ -n "${SFTP_PORT}" ]]; then
            SFTP_CREATE_CMD+=(--port="${SFTP_PORT}")
        fi
        if [[ -n "${SFTP_PASSWORD}" ]]; then
            SFTP_CREATE_CMD+=(--password="${SFTP_PASSWORD}")
        fi
        if [[ -n "${SFTP_KEY_FILE}" ]]; then
            SFTP_CREATE_CMD+=(--keyfile="${SFTP_KEY_FILE}")
        fi
        add_user_overrides SFTP_CREATE_CMD
        "${SFTP_CREATE_CMD[@]}"
    elif [[ -n "${RCLONE_REMOTE_PATH}" ]]; then
        echo "Creating Rclone repository"
        RCLONE_CREATE_CMD=("${KOPIA[@]}" repository create rclone \
            --remote-path="${RCLONE_REMOTE_PATH}")
        if [[ -n "${RCLONE_EXE}" ]]; then
            RCLONE_CREATE_CMD+=(--rclone-exe="${RCLONE_EXE}")
        fi
        if [[ -n "${RCLONE_CONFIG}" ]]; then
            RCLONE_CREATE_CMD+=(--rclone-config="${RCLONE_CONFIG}")
        fi
        add_user_overrides RCLONE_CREATE_CMD
        "${RCLONE_CREATE_CMD[@]}"
    elif [[ -n "${GOOGLE_DRIVE_FOLDER_ID}" ]]; then
        echo "Creating Google Drive repository"
        GDRIVE_CREATE_CMD=("${KOPIA[@]}" repository create gdrive \
            --folder-id="${GOOGLE_DRIVE_FOLDER_ID}" \
            --credentials-file="${GOOGLE_DRIVE_CREDENTIALS}")
        add_user_overrides GDRIVE_CREATE_CMD
        "${GDRIVE_CREATE_CMD[@]}"
    else
        error 1 "No repository configuration found"
    fi
}

function do_backup {
    echo "=== Starting backup ==="
    
    # Build snapshot command with options
    declare -a SNAPSHOT_CMD
    SNAPSHOT_CMD=("${KOPIA[@]}" "snapshot" "create" "${DATA_DIR}")
    
    # Add compression algorithm if specified
    # Note: compression is typically set at repository level during creation
    # For now, skipping per-snapshot compression to avoid compatibility issues
    
    # Add parallelism if specified
    if [[ -n "${KOPIA_PARALLELISM}" ]]; then
        SNAPSHOT_CMD+=(--parallel="${KOPIA_PARALLELISM}")
    fi
    
    # Add source path override if specified
    if [[ -n "${KOPIA_SOURCE_PATH_OVERRIDE}" ]]; then
        echo "Using source path override: ${KOPIA_SOURCE_PATH_OVERRIDE}"
        SNAPSHOT_CMD+=(--override-source="${KOPIA_SOURCE_PATH_OVERRIDE}")
    fi
    
    # Run before-snapshot action if specified
    if [[ -n "${KOPIA_BEFORE_SNAPSHOT}" ]]; then
        if ! execute_action "${KOPIA_BEFORE_SNAPSHOT}" "before-snapshot"; then
            error 1 "Before-snapshot action failed"
        fi
    fi
    
    # Create snapshot with error handling
    if ! "${SNAPSHOT_CMD[@]}"; then
        error 1 "Failed to create snapshot"
    fi
    
    echo "Snapshot created successfully"
    
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
    if ! "${KOPIA[@]}" maintenance run --full; then
        echo "Warning: Maintenance operation failed, but continuing"
        # Don't fail the entire operation for maintenance issues
        return 0
    fi
    echo "Maintenance completed successfully"
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
        if ! "${POLICY_CMD[@]}"; then
            echo "Warning: Failed to set retention policy, continuing with defaults"
            # Don't fail the entire operation for policy setting issues
        else
            echo "Retention policy applied successfully"
        fi
    else
        echo "No retention policy settings specified, using defaults"
    fi
}

function select_snapshot_to_restore {
    echo "Selecting snapshot to restore"
    
    # List snapshots for the specific data directory path
    # This ensures we get snapshots for the correct username@hostname:/path
    local snapshot_list_cmd=("${KOPIA[@]}" snapshot list "${DATA_DIR}" --json)
    
    # List snapshots and find the appropriate one
    if [[ -n "${KOPIA_RESTORE_AS_OF}" ]]; then
        echo "Restoring as of: ${KOPIA_RESTORE_AS_OF}"
        "${snapshot_list_cmd[@]}" | jq -r ".[] | select(.startTime <= \"${KOPIA_RESTORE_AS_OF}\") | .id" | head -1
    elif [[ -n "${KOPIA_SHALLOW}" ]]; then
        echo "Shallow restore, showing last ${KOPIA_SHALLOW} snapshots"
        "${snapshot_list_cmd[@]}" | jq -r ".[0:${KOPIA_SHALLOW}][] | .id" | head -1
    else
        # Get latest snapshot
        "${snapshot_list_cmd[@]}" | jq -r ".[0].id"
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
        return 0
    fi
    
    echo "Selected snapshot with id: ${snapshot_id}"
    
    # Restore the snapshot with proper error handling
    if ! pushd "${DATA_DIR}" >/dev/null; then
        error 1 "Failed to change to data directory: ${DATA_DIR}"
    fi
    
    if ! "${KOPIA[@]}" snapshot restore "${snapshot_id}" .; then
        popd >/dev/null || true
        error 1 "Failed to restore snapshot: ${snapshot_id}"
    fi
    
    if ! popd >/dev/null; then
        echo "Warning: Failed to return to original directory"
    fi
    
    echo "Snapshot restore completed successfully"
}

echo "Testing mandatory env variables"
# Check the mandatory env variables
for var in KOPIA_PASSWORD \
           DATA_DIR \
           ; do
    check_var_defined $var
done

# Validate cache directory is writable
if [[ ! -w "${KOPIA_CACHE_DIR}" ]]; then
    error 1 "Cache directory ${KOPIA_CACHE_DIR} is not writable"
fi

echo "Cache directory validation passed"
echo ""

# Cache directory already configured above

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
    # Execute operations based on arguments (current VolSync approach)
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