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
echo "KOPIA_REPOSITORY: $([ -n "${KOPIA_REPOSITORY}" ] && echo "[SET]" || echo "[NOT SET]")"
echo "KOPIA_PASSWORD: $([ -n "${KOPIA_PASSWORD}" ] && echo "[SET]" || echo "[NOT SET]")"
echo "KOPIA_S3_BUCKET: $([ -n "${KOPIA_S3_BUCKET}" ] && echo "[SET]" || echo "[NOT SET]")"
echo "KOPIA_S3_ENDPOINT: $([ -n "${KOPIA_S3_ENDPOINT}" ] && echo "[SET]" || echo "[NOT SET]")"
echo "AWS_S3_ENDPOINT: $([ -n "${AWS_S3_ENDPOINT}" ] && echo "[SET]" || echo "[NOT SET]")"
echo "KOPIA_S3_DISABLE_TLS: $([ -n "${KOPIA_S3_DISABLE_TLS}" ] && echo "[SET]" || echo "[NOT SET]")"
echo "AWS_S3_DISABLE_TLS: $([ -n "${AWS_S3_DISABLE_TLS}" ] && echo "[SET]" || echo "[NOT SET]")"
echo "AWS_ACCESS_KEY_ID: $([ -n "${AWS_ACCESS_KEY_ID}" ] && echo "[SET]" || echo "[NOT SET]")"
echo "AWS_SECRET_ACCESS_KEY: $([ -n "${AWS_SECRET_ACCESS_KEY}" ] && echo "[SET]" || echo "[NOT SET]")"
echo "AWS_DEFAULT_REGION: $([ -n "${AWS_DEFAULT_REGION}" ] && echo "[SET]" || echo "[NOT SET]")"
echo "AWS_REGION: $([ -n "${AWS_REGION}" ] && echo "[SET]" || echo "[NOT SET]")"
echo "KOPIA_OVERRIDE_USERNAME: $([ -n "${KOPIA_OVERRIDE_USERNAME}" ] && echo "[SET]" || echo "[NOT SET]")"
echo "KOPIA_OVERRIDE_HOSTNAME: $([ -n "${KOPIA_OVERRIDE_HOSTNAME}" ] && echo "[SET]" || echo "[NOT SET]")"
echo "KOPIA_SOURCE_PATH_OVERRIDE: $([ -n "${KOPIA_SOURCE_PATH_OVERRIDE}" ] && echo "[SET]" || echo "[NOT SET]")"
echo "KOPIA_MANUAL_CONFIG: $([ -n "${KOPIA_MANUAL_CONFIG}" ] && echo "[SET]" || echo "[NOT SET]")"
echo "KOPIA_RESTORE_AS_OF: $([ -n "${KOPIA_RESTORE_AS_OF}" ] && echo "[SET]" || echo "[NOT SET]")"
echo "KOPIA_SHALLOW: $([ -n "${KOPIA_SHALLOW}" ] && echo "[SET]" || echo "[NOT SET]")"
echo "KOPIA_PREVIOUS: $([ -n "${KOPIA_PREVIOUS}" ] && echo "[SET]" || echo "[NOT SET]")"
echo ""
echo "=== Additional Backend Environment Variables ==="
echo "KOPIA_B2_BUCKET: $([ -n "${KOPIA_B2_BUCKET}" ] && echo "[SET]" || echo "[NOT SET]")"
echo "B2_ACCOUNT_ID: $([ -n "${B2_ACCOUNT_ID}" ] && echo "[SET]" || echo "[NOT SET]")"
echo "B2_APPLICATION_KEY: $([ -n "${B2_APPLICATION_KEY}" ] && echo "[SET]" || echo "[NOT SET]")"
echo "WEBDAV_URL: $([ -n "${WEBDAV_URL}" ] && echo "[SET]" || echo "[NOT SET]")"
echo "WEBDAV_USERNAME: $([ -n "${WEBDAV_USERNAME}" ] && echo "[SET]" || echo "[NOT SET]")"
echo "WEBDAV_PASSWORD: $([ -n "${WEBDAV_PASSWORD}" ] && echo "[SET]" || echo "[NOT SET]")"
echo "SFTP_HOST: $([ -n "${SFTP_HOST}" ] && echo "[SET]" || echo "[NOT SET]")"
echo "SFTP_PORT: $([ -n "${SFTP_PORT}" ] && echo "[SET]" || echo "[NOT SET]")"
echo "SFTP_USERNAME: $([ -n "${SFTP_USERNAME}" ] && echo "[SET]" || echo "[NOT SET]")"
echo "SFTP_PASSWORD: $([ -n "${SFTP_PASSWORD}" ] && echo "[SET]" || echo "[NOT SET]")"
echo "SFTP_PATH: $([ -n "${SFTP_PATH}" ] && echo "[SET]" || echo "[NOT SET]")"
echo "SFTP_KEY_FILE: $([ -n "${SFTP_KEY_FILE}" ] && echo "[SET]" || echo "[NOT SET]")"
echo "RCLONE_REMOTE_PATH: $([ -n "${RCLONE_REMOTE_PATH}" ] && echo "[SET]" || echo "[NOT SET]")"
echo "RCLONE_EXE: $([ -n "${RCLONE_EXE}" ] && echo "[SET]" || echo "[NOT SET]")"
echo "RCLONE_CONFIG: $([ -n "${RCLONE_CONFIG}" ] && echo "[SET]" || echo "[NOT SET]")"
echo "GOOGLE_DRIVE_FOLDER_ID: $([ -n "${GOOGLE_DRIVE_FOLDER_ID}" ] && echo "[SET]" || echo "[NOT SET]")"
echo "GOOGLE_DRIVE_CREDENTIALS: $([ -n "${GOOGLE_DRIVE_CREDENTIALS}" ] && echo "[SET]" || echo "[NOT SET]")"
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
    echo "=== Checking directory for content ==="
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

# Apply manual repository configuration from JSON if provided
function apply_manual_config {
    if [[ -n "${KOPIA_MANUAL_CONFIG}" ]]; then
        echo "=== Applying manual repository configuration ==="
        echo "Manual configuration provided, parsing JSON..."
        
        # Validate JSON syntax first
        if ! echo "${KOPIA_MANUAL_CONFIG}" | jq . >/dev/null 2>&1; then
            echo "ERROR: Invalid JSON in KOPIA_MANUAL_CONFIG"
            echo "Falling back to automatic configuration"
            return 1
        fi
        
        # Parse and validate manual configuration
        local manual_config="${KOPIA_MANUAL_CONFIG}"
        
        # Extract configuration sections
        local encryption_config
        local compression_config
        local splitter_config
        local caching_config
        
        encryption_config=$(echo "${manual_config}" | jq -r '.encryption // empty')
        compression_config=$(echo "${manual_config}" | jq -r '.compression // empty')
        splitter_config=$(echo "${manual_config}" | jq -r '.splitter // empty')
        caching_config=$(echo "${manual_config}" | jq -r '.caching // empty')
        
        echo "Parsed configuration sections:"
        echo "  Encryption: ${encryption_config:+[SET]}${encryption_config:-[NOT SET]}"
        echo "  Compression: ${compression_config:+[SET]}${compression_config:-[NOT SET]}"
        echo "  Splitter: ${splitter_config:+[SET]}${splitter_config:-[NOT SET]}"
        echo "  Caching: ${caching_config:+[SET]}${caching_config:-[NOT SET]}"
        
        # Store manual configuration for repository creation/connection
        export KOPIA_MANUAL_ENCRYPTION_CONFIG="${encryption_config}"
        export KOPIA_MANUAL_COMPRESSION_CONFIG="${compression_config}"
        export KOPIA_MANUAL_SPLITTER_CONFIG="${splitter_config}"
        export KOPIA_MANUAL_CACHING_CONFIG="${caching_config}"
        
        echo "Manual configuration parsed and exported successfully"
        echo "Configuration will be applied during repository creation/connection"
        return 0
    else
        echo "No manual configuration provided, using automatic settings"
        return 0
    fi
}

# Add manual configuration parameters to repository creation command
# add_manual_config_params command_array_name
function add_manual_config_params {
    local -n cmd_array=$1
    
    if [[ -n "${KOPIA_MANUAL_CONFIG}" ]]; then
        echo "Applying manual configuration parameters to repository command..."
        
        # Apply encryption configuration
        if [[ -n "${KOPIA_MANUAL_ENCRYPTION_CONFIG}" && "${KOPIA_MANUAL_ENCRYPTION_CONFIG}" != "null" ]]; then
            local encryption_algorithm
            encryption_algorithm=$(echo "${KOPIA_MANUAL_ENCRYPTION_CONFIG}" | jq -r '.algorithm // empty')
            
            if [[ -n "${encryption_algorithm}" && "${encryption_algorithm}" != "null" ]]; then
                # Validate encryption algorithm against allowed values
                case "${encryption_algorithm}" in
                    "CHACHA20-POLY1305"|"AES256-GCM"|"AES192-GCM"|"AES128-GCM")
                        echo "  Using encryption algorithm: ${encryption_algorithm}"
                        cmd_array+=(--encryption="${encryption_algorithm}")
                        ;;
                    *)
                        echo "  WARNING: Unsupported encryption algorithm '${encryption_algorithm}', using default"
                        ;;
                esac
            fi
        fi
        
        # Apply compression configuration
        if [[ -n "${KOPIA_MANUAL_COMPRESSION_CONFIG}" && "${KOPIA_MANUAL_COMPRESSION_CONFIG}" != "null" ]]; then
            local compression_algorithm
            local compression_min_size
            local compression_max_size
            
            compression_algorithm=$(echo "${KOPIA_MANUAL_COMPRESSION_CONFIG}" | jq -r '.algorithm // empty')
            compression_min_size=$(echo "${KOPIA_MANUAL_COMPRESSION_CONFIG}" | jq -r '.minSize // empty')
            compression_max_size=$(echo "${KOPIA_MANUAL_COMPRESSION_CONFIG}" | jq -r '.maxSize // empty')
            
            if [[ -n "${compression_algorithm}" && "${compression_algorithm}" != "null" ]]; then
                # Validate compression algorithm against common Kopia options
                case "${compression_algorithm}" in
                    "ZSTD-FASTEST"|"ZSTD-FAST"|"ZSTD-DEFAULT"|"ZSTD-BETTER"|"ZSTD-BEST"|"S2-DEFAULT"|"S2-BETTER"|"S2-BEST"|"DEFLATE-DEFAULT"|"DEFLATE-BEST-SPEED"|"DEFLATE-BEST-COMPRESSION"|"none")
                        echo "  Using compression algorithm: ${compression_algorithm}"
                        cmd_array+=(--compression="${compression_algorithm}")
                        ;;
                    *)
                        echo "  WARNING: Unsupported compression algorithm '${compression_algorithm}', using default"
                        ;;
                esac
            fi
            
            if [[ -n "${compression_min_size}" && "${compression_min_size}" != "null" ]]; then
                if [[ "${compression_min_size}" =~ ^[0-9]+$ ]]; then
                    echo "  Using compression minimum size: ${compression_min_size}"
                    cmd_array+=(--compression-min-size="${compression_min_size}")
                else
                    echo "  WARNING: Invalid compression min size '${compression_min_size}', must be numeric"
                fi
            fi
            
            if [[ -n "${compression_max_size}" && "${compression_max_size}" != "null" ]]; then
                if [[ "${compression_max_size}" =~ ^[0-9]+$ ]]; then
                    echo "  Using compression maximum size: ${compression_max_size}"
                    cmd_array+=(--compression-max-size="${compression_max_size}")
                else
                    echo "  WARNING: Invalid compression max size '${compression_max_size}', must be numeric"
                fi
            fi
        fi
        
        # Apply splitter configuration
        if [[ -n "${KOPIA_MANUAL_SPLITTER_CONFIG}" && "${KOPIA_MANUAL_SPLITTER_CONFIG}" != "null" ]]; then
            local splitter_algorithm
            splitter_algorithm=$(echo "${KOPIA_MANUAL_SPLITTER_CONFIG}" | jq -r '.algorithm // empty')
            
            if [[ -n "${splitter_algorithm}" && "${splitter_algorithm}" != "null" ]]; then
                # Validate splitter algorithm against common Kopia options
                case "${splitter_algorithm}" in
                    "DYNAMIC-4M-BUZHASH"|"DYNAMIC-8M-BUZHASH"|"DYNAMIC-16M-BUZHASH"|"DYNAMIC-32M-BUZHASH"|"FIXED-1M"|"FIXED-2M"|"FIXED-4M"|"FIXED-8M"|"FIXED-16M"|"FIXED-32M")
                        echo "  Using splitter algorithm: ${splitter_algorithm}"
                        cmd_array+=(--object-splitter="${splitter_algorithm}")
                        ;;
                    *)
                        echo "  WARNING: Unsupported splitter algorithm '${splitter_algorithm}', using default"
                        ;;
                esac
            fi
        fi
        
        # Note: Caching configuration is typically applied post-repository creation
        # via 'kopia cache set' commands, which we handle in ensure_connected
        
        echo "Manual configuration parameters applied to repository command"
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

# Apply JSON repository configuration using kopia's native support
function apply_json_repository_config {
    if [[ -n "${KOPIA_STRUCTURED_REPOSITORY_CONFIG}" ]]; then
        echo "=== Applying JSON repository configuration ==="
        
        # Validate JSON syntax
        if ! echo "${KOPIA_STRUCTURED_REPOSITORY_CONFIG}" | jq . >/dev/null 2>&1; then
            echo "ERROR: Invalid JSON in KOPIA_STRUCTURED_REPOSITORY_CONFIG"
            return 1
        fi
        
        echo "JSON configuration validated"
        
        # Write JSON to temp file for kopia native consumption
        local temp_config="/tmp/kopia-repo-config.json"
        echo "${KOPIA_STRUCTURED_REPOSITORY_CONFIG}" > "${temp_config}"
        chmod 600 "${temp_config}"  # Secure the config file
        
        echo "JSON configuration ready for kopia native consumption ($(wc -c < "${temp_config}") bytes)"
        
        # Set environment variable for connection logic to use
        export KOPIA_JSON_CONFIG_FILE="${temp_config}"
        
        echo "=== JSON configuration prepared ==="
    fi
}

# Connect to or create the repository
function ensure_connected {
    echo "=== Connecting to repository ==="
    
    # Apply JSON repository configuration first
    apply_json_repository_config
    
    # Apply manual configuration (parses JSON and sets environment variables)
    apply_manual_config
    
    # Try to connect to existing repository (let errors display naturally)
    if ! timeout 10s "${KOPIA[@]}" repository status >/dev/null 2>&1; then
        echo "Repository not connected, attempting to connect or create..."
        echo ""
        
        # Disable exit on error for connection attempts
        set +e
        
        # Try JSON config file first if available
        if [[ -n "${KOPIA_JSON_CONFIG_FILE}" && -f "${KOPIA_JSON_CONFIG_FILE}" ]]; then
            echo "Attempting to connect using JSON configuration file..."
            "${KOPIA[@]}" repository connect from-config --file="${KOPIA_JSON_CONFIG_FILE}"
            local json_result=$?
            
            if [[ $json_result -ne 0 ]]; then
                echo "JSON config connection failed, trying other methods..."
                echo ""
            else
                echo "JSON config connection successful"
                set -e  # Re-enable exit on error
                return 0
            fi
        fi
        
        # Try to connect from legacy config file
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
    declare -a CACHE_CMD
    CACHE_CMD=("${KOPIA[@]}" cache set --cache-directory="${KOPIA_CACHE_DIR}")
    
    # Apply manual cache configuration if specified
    if [[ -n "${KOPIA_MANUAL_CACHING_CONFIG}" && "${KOPIA_MANUAL_CACHING_CONFIG}" != "null" ]]; then
        echo "Applying manual cache configuration..."
        
        local max_cache_size
        max_cache_size=$(echo "${KOPIA_MANUAL_CACHING_CONFIG}" | jq -r '.maxCacheSize // empty')
        
        if [[ -n "${max_cache_size}" && "${max_cache_size}" != "null" ]]; then
            if [[ "${max_cache_size}" =~ ^[0-9]+$ ]]; then
                echo "  Using maximum cache size: ${max_cache_size} bytes"
                CACHE_CMD+=(--max-size="${max_cache_size}")
            else
                echo "  WARNING: Invalid cache max size '${max_cache_size}', must be numeric in bytes"
            fi
        fi
    fi
    
    if ! "${CACHE_CMD[@]}"; then
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
    # Check both explicit KOPIA_S3_BUCKET and s3:// repository URL pattern
    if [[ -n "${KOPIA_S3_BUCKET}" ]] || [[ "${KOPIA_REPOSITORY}" =~ ^s3:// ]]; then
        echo "Connecting to S3 repository"
        echo ""
        echo "=== S3 Connection Debug ==="
        echo "KOPIA_S3_BUCKET: $([ -n "${KOPIA_S3_BUCKET}" ] && echo "[SET]" || echo "[NOT SET]")"
        echo "KOPIA_S3_ENDPOINT: $([ -n "${KOPIA_S3_ENDPOINT}" ] && echo "[SET]" || echo "[NOT SET]")"
        echo "AWS_S3_ENDPOINT: $([ -n "${AWS_S3_ENDPOINT}" ] && echo "[SET]" || echo "[NOT SET]")"
        echo "AWS_ACCESS_KEY_ID: $([ -n "${AWS_ACCESS_KEY_ID}" ] && echo "[SET]" || echo "[NOT SET]")"
        echo "AWS_SECRET_ACCESS_KEY: $([ -n "${AWS_SECRET_ACCESS_KEY}" ] && echo "[SET]" || echo "[NOT SET]")"
        echo "KOPIA_REPOSITORY: $([ -n "${KOPIA_REPOSITORY}" ] && echo "[SET]" || echo "[NOT SET]")"
        echo "KOPIA_S3_DISABLE_TLS: $([ -n "${KOPIA_S3_DISABLE_TLS}" ] && echo "[SET]" || echo "[NOT SET]")"
        echo "AWS_S3_DISABLE_TLS: $([ -n "${AWS_S3_DISABLE_TLS}" ] && echo "[SET]" || echo "[NOT SET]")"
        echo "KOPIA_PASSWORD: $([ -n "${KOPIA_PASSWORD}" ] && echo "[SET]" || echo "[NOT SET]")"
        
        # Extract bucket name from repository URL if not explicitly set
        local S3_BUCKET="${KOPIA_S3_BUCKET}"
        if [[ -z "${S3_BUCKET}" ]] && [[ "${KOPIA_REPOSITORY}" =~ ^s3://([a-z0-9][a-z0-9.-]{1,61}[a-z0-9])/?(.*)$ ]]; then
            S3_BUCKET="${BASH_REMATCH[1]}"
            # Validate S3 bucket name format
            if [[ "${S3_BUCKET}" =~ \.\. ]] || [[ "${S3_BUCKET}" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
                echo "ERROR: Invalid S3 bucket name format"
                return 1
            fi
            echo "Extracted S3 bucket from repository URL: ${S3_BUCKET}"
        fi
        
        if [[ -z "${S3_BUCKET}" ]]; then
            echo "ERROR: Could not determine S3 bucket name"
            return 1
        fi
        
        # Use KOPIA_S3_ENDPOINT if set, otherwise fall back to AWS_S3_ENDPOINT, otherwise use default
        local S3_ENDPOINT="${KOPIA_S3_ENDPOINT:-${AWS_S3_ENDPOINT:-s3.amazonaws.com}}"
        
        # Strip protocol from endpoint if present (Kopia expects hostname only)
        S3_ENDPOINT=$(echo "${S3_ENDPOINT}" | sed 's|^https\?://||')
        echo "Resolved S3_ENDPOINT: ${S3_ENDPOINT}"
        
        S3_CONNECT_CMD=("${KOPIA[@]}" repository connect s3)
        
        # Add required parameters with validation
        if [[ -n "${S3_BUCKET}" ]]; then
            S3_CONNECT_CMD+=(--bucket="${S3_BUCKET}")
        fi
        
        if [[ -n "${S3_ENDPOINT}" ]]; then
            S3_CONNECT_CMD+=(--endpoint="${S3_ENDPOINT}")
        fi
        
        if [[ -n "${AWS_ACCESS_KEY_ID}" ]]; then
            S3_CONNECT_CMD+=(--access-key="${AWS_ACCESS_KEY_ID}")
        fi
        
        if [[ -n "${AWS_SECRET_ACCESS_KEY}" ]]; then
            S3_CONNECT_CMD+=(--secret-access-key="${AWS_SECRET_ACCESS_KEY}")
        fi
        
        # Add optional AWS region (support both naming conventions)
        local AWS_REGION_VALUE="${AWS_REGION:-${AWS_DEFAULT_REGION}}"
        if [[ -n "${AWS_REGION_VALUE}" ]]; then
            S3_CONNECT_CMD+=(--region="${AWS_REGION_VALUE}")
        fi
        
        # Add optional AWS session token
        if [[ -n "${AWS_SESSION_TOKEN}" ]]; then
            S3_CONNECT_CMD+=(--session-token="${AWS_SESSION_TOKEN}")
        fi
        
        # Extract prefix from KOPIA_REPOSITORY (e.g., s3://bucket/prefix -> prefix)
        if [[ "${KOPIA_REPOSITORY}" =~ s3://[^/]+/(.+) ]]; then
            S3_PREFIX="${BASH_REMATCH[1]}"
            # Validate S3 prefix for security
            if [[ "${S3_PREFIX}" =~ ^[a-zA-Z0-9._/-]+$ ]] && [[ ! "${S3_PREFIX}" =~ \.\. ]]; then
                # Ensure S3 prefix has a trailing slash for proper path separation
                # This prevents ambiguous file paths in S3 storage
                if [[ -n "${S3_PREFIX}" ]] && [[ ! "${S3_PREFIX}" =~ /$ ]]; then
                    S3_PREFIX="${S3_PREFIX}/"
                    echo "Added trailing slash to S3 prefix for proper path separation"
                fi
                echo "Using S3 prefix: ${S3_PREFIX}"
                if [[ -n "${S3_PREFIX}" ]]; then
                    S3_CONNECT_CMD+=(--prefix="${S3_PREFIX}")
                fi
            else
                echo "ERROR: Invalid S3 prefix format. Only alphanumeric, dots, dashes, underscores and forward slashes allowed"
                return 1
            fi
        else
            echo "No S3 prefix detected in KOPIA_REPOSITORY"
        fi
        
        # Add disable TLS flag if specified (support both naming conventions)
        if [[ "${KOPIA_S3_DISABLE_TLS}" == "true" ]] || [[ "${AWS_S3_DISABLE_TLS}" == "true" ]]; then
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
    # Check both explicit KOPIA_S3_BUCKET and s3:// repository URL pattern
    if [[ -n "${KOPIA_S3_BUCKET}" ]] || [[ "${KOPIA_REPOSITORY}" =~ ^s3:// ]]; then
        echo "Creating S3 repository"
        echo ""
        echo "=== S3 Creation Debug ==="
        echo "KOPIA_S3_BUCKET: $([ -n "${KOPIA_S3_BUCKET}" ] && echo "[SET]" || echo "[NOT SET]")"
        echo "KOPIA_S3_ENDPOINT: $([ -n "${KOPIA_S3_ENDPOINT}" ] && echo "[SET]" || echo "[NOT SET]")"
        echo "AWS_S3_ENDPOINT: $([ -n "${AWS_S3_ENDPOINT}" ] && echo "[SET]" || echo "[NOT SET]")"
        echo "AWS_ACCESS_KEY_ID: $([ -n "${AWS_ACCESS_KEY_ID}" ] && echo "[SET]" || echo "[NOT SET]")"
        echo "AWS_SECRET_ACCESS_KEY: $([ -n "${AWS_SECRET_ACCESS_KEY}" ] && echo "[SET]" || echo "[NOT SET]")"
        echo "KOPIA_REPOSITORY: $([ -n "${KOPIA_REPOSITORY}" ] && echo "[SET]" || echo "[NOT SET]")"
        echo "KOPIA_S3_DISABLE_TLS: $([ -n "${KOPIA_S3_DISABLE_TLS}" ] && echo "[SET]" || echo "[NOT SET]")"
        echo "AWS_S3_DISABLE_TLS: $([ -n "${AWS_S3_DISABLE_TLS}" ] && echo "[SET]" || echo "[NOT SET]")"
        echo "KOPIA_PASSWORD: $([ -n "${KOPIA_PASSWORD}" ] && echo "[SET]" || echo "[NOT SET]")"
        echo "KOPIA_CACHE_DIR: $([ -n "${KOPIA_CACHE_DIR}" ] && echo "[SET]" || echo "[NOT SET]")"
        
        # Extract bucket name from repository URL if not explicitly set
        local S3_BUCKET="${KOPIA_S3_BUCKET}"
        if [[ -z "${S3_BUCKET}" ]] && [[ "${KOPIA_REPOSITORY}" =~ ^s3://([a-z0-9][a-z0-9.-]{1,61}[a-z0-9])/?(.*)$ ]]; then
            S3_BUCKET="${BASH_REMATCH[1]}"
            # Validate S3 bucket name format
            if [[ "${S3_BUCKET}" =~ \.\. ]] || [[ "${S3_BUCKET}" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
                echo "ERROR: Invalid S3 bucket name format"
                return 1
            fi
            echo "Extracted S3 bucket from repository URL: ${S3_BUCKET}"
        fi
        
        if [[ -z "${S3_BUCKET}" ]]; then
            echo "ERROR: Could not determine S3 bucket name"
            return 1
        fi
        
        # Use KOPIA_S3_ENDPOINT if set, otherwise fall back to AWS_S3_ENDPOINT, otherwise use default
        local S3_ENDPOINT="${KOPIA_S3_ENDPOINT:-${AWS_S3_ENDPOINT:-s3.amazonaws.com}}"
        
        # Strip protocol from endpoint if present (Kopia expects hostname only)
        S3_ENDPOINT=$(echo "${S3_ENDPOINT}" | sed 's|^https\?://||')
        echo "Resolved S3_ENDPOINT: ${S3_ENDPOINT}"
        
        S3_CREATE_CMD=("${KOPIA[@]}" repository create s3)
        
        # Add required parameters with validation
        if [[ -n "${S3_BUCKET}" ]]; then
            S3_CREATE_CMD+=(--bucket="${S3_BUCKET}")
        fi
        
        if [[ -n "${S3_ENDPOINT}" ]]; then
            S3_CREATE_CMD+=(--endpoint="${S3_ENDPOINT}")
        fi
        
        if [[ -n "${AWS_ACCESS_KEY_ID}" ]]; then
            S3_CREATE_CMD+=(--access-key="${AWS_ACCESS_KEY_ID}")
        fi
        
        if [[ -n "${AWS_SECRET_ACCESS_KEY}" ]]; then
            S3_CREATE_CMD+=(--secret-access-key="${AWS_SECRET_ACCESS_KEY}")
        fi
        
        if [[ -n "${KOPIA_CACHE_DIR}" ]]; then
            S3_CREATE_CMD+=(--cache-directory="${KOPIA_CACHE_DIR}")
        fi
        
        # Add optional AWS region (support both naming conventions)
        local AWS_REGION_VALUE="${AWS_REGION:-${AWS_DEFAULT_REGION}}"
        if [[ -n "${AWS_REGION_VALUE}" ]]; then
            S3_CREATE_CMD+=(--region="${AWS_REGION_VALUE}")
        fi
        
        # Add optional AWS session token
        if [[ -n "${AWS_SESSION_TOKEN}" ]]; then
            S3_CREATE_CMD+=(--session-token="${AWS_SESSION_TOKEN}")
        fi
        
        # Extract prefix from KOPIA_REPOSITORY (e.g., s3://bucket/prefix -> prefix)
        if [[ "${KOPIA_REPOSITORY}" =~ s3://[^/]+/(.+) ]]; then
            S3_PREFIX="${BASH_REMATCH[1]}"
            # Validate S3 prefix for security
            if [[ "${S3_PREFIX}" =~ ^[a-zA-Z0-9._/-]+$ ]] && [[ ! "${S3_PREFIX}" =~ \.\. ]]; then
                # Ensure S3 prefix has a trailing slash for proper path separation
                # This prevents ambiguous file paths in S3 storage
                if [[ -n "${S3_PREFIX}" ]] && [[ ! "${S3_PREFIX}" =~ /$ ]]; then
                    S3_PREFIX="${S3_PREFIX}/"
                    echo "Added trailing slash to S3 prefix for proper path separation"
                fi
                echo "Using S3 prefix: ${S3_PREFIX}"
                if [[ -n "${S3_PREFIX}" ]]; then
                    S3_CREATE_CMD+=(--prefix="${S3_PREFIX}")
                fi
            else
                echo "ERROR: Invalid S3 prefix format. Only alphanumeric, dots, dashes, underscores and forward slashes allowed"
                return 1
            fi
        else
            echo "No S3 prefix detected in KOPIA_REPOSITORY"
        fi
        
        # Add disable TLS flag if specified (support both naming conventions)
        if [[ "${KOPIA_S3_DISABLE_TLS}" == "true" ]] || [[ "${AWS_S3_DISABLE_TLS}" == "true" ]]; then
            S3_CREATE_CMD+=(--disable-tls)
        fi
        
        # Add username/hostname overrides if specified
        add_user_overrides S3_CREATE_CMD
        
        # Add manual configuration parameters if specified
        add_manual_config_params S3_CREATE_CMD
        
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
        add_manual_config_params AZURE_CREATE_CMD
        "${AZURE_CREATE_CMD[@]}"
    elif [[ -n "${KOPIA_GCS_BUCKET}" ]]; then
        echo "Creating GCS repository"
        GCS_CREATE_CMD=("${KOPIA[@]}" repository create gcs \
            --bucket="${KOPIA_GCS_BUCKET}" \
            --credentials-file="${GOOGLE_APPLICATION_CREDENTIALS}")
        add_user_overrides GCS_CREATE_CMD
        add_manual_config_params GCS_CREATE_CMD
        "${GCS_CREATE_CMD[@]}"
    elif [[ -n "${KOPIA_FS_PATH}" ]]; then
        echo "Creating filesystem repository"
        FS_CREATE_CMD=("${KOPIA[@]}" repository create filesystem --path="${KOPIA_FS_PATH}")
        add_user_overrides FS_CREATE_CMD
        add_manual_config_params FS_CREATE_CMD
        "${FS_CREATE_CMD[@]}"
    elif [[ -n "${KOPIA_B2_BUCKET}" ]]; then
        echo "Creating Backblaze B2 repository"
        B2_CREATE_CMD=("${KOPIA[@]}" repository create b2 \
            --bucket="${KOPIA_B2_BUCKET}" \
            --key-id="${B2_ACCOUNT_ID}" \
            --key="${B2_APPLICATION_KEY}")
        add_user_overrides B2_CREATE_CMD
        add_manual_config_params B2_CREATE_CMD
        "${B2_CREATE_CMD[@]}"
    elif [[ -n "${WEBDAV_URL}" ]]; then
        echo "Creating WebDAV repository"
        WEBDAV_CREATE_CMD=("${KOPIA[@]}" repository create webdav \
            --url="${WEBDAV_URL}" \
            --username="${WEBDAV_USERNAME}" \
            --password="${WEBDAV_PASSWORD}")
        add_user_overrides WEBDAV_CREATE_CMD
        add_manual_config_params WEBDAV_CREATE_CMD
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
        add_manual_config_params SFTP_CREATE_CMD
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
        add_manual_config_params RCLONE_CREATE_CMD
        "${RCLONE_CREATE_CMD[@]}"
    elif [[ -n "${GOOGLE_DRIVE_FOLDER_ID}" ]]; then
        echo "Creating Google Drive repository"
        GDRIVE_CREATE_CMD=("${KOPIA[@]}" repository create gdrive \
            --folder-id="${GOOGLE_DRIVE_FOLDER_ID}" \
            --credentials-file="${GOOGLE_DRIVE_CREDENTIALS}")
        add_user_overrides GDRIVE_CREATE_CMD
        add_manual_config_params GDRIVE_CREATE_CMD
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
    
    # Create snapshot with error handling - ensure real-time progress output
    # Execute with explicit file descriptor handling to ensure real-time output
    echo "Snapshotting ${KOPIA_OVERRIDE_USERNAME:-$(whoami)}@${KOPIA_OVERRIDE_HOSTNAME:-$(hostname)}:${KOPIA_SOURCE_PATH_OVERRIDE:-$DATA_DIR} ..."
    if ! "${SNAPSHOT_CMD[@]}" </dev/null; then
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

# Function to discover available snapshots in the repository
function discover_available_snapshots {
    echo ""
    echo "=== Discovery Mode: Available Snapshots ==="
    
    # List all snapshots in the repository (not just for our path)
    echo "Listing all available snapshot identities in the repository..."
    
    # Use kopia snapshot list without path to see all snapshots
    # Output in JSON for parsing by the controller
    local all_snapshots
    all_snapshots=$("${KOPIA[@]}" snapshot list --all --json 2>/dev/null || true)
    
    if [[ -n "${all_snapshots}" ]] && [[ "${all_snapshots}" != "[]" ]]; then
        echo "Found snapshots in repository:"
        echo ""
        
        # Output raw JSON for controller parsing
        echo "${all_snapshots}" | jq -c '.[] | {id: .id, userName: .source.userName, hostName: .source.host, path: .source.path, startTime: .startTime, endTime: .endTime}' 2>/dev/null || true
        
        echo ""
        echo "Available identities (username@hostname combinations):"
        # Also provide human-readable summary
        echo "${all_snapshots}" | jq -r '.[] | "\(.source.userName)@\(.source.host):\(.source.path) - Last snapshot: \(.endTime)"' | sort -u 2>/dev/null || true
    else
        echo "No snapshots found in the repository"
        
        # Try to list repository manifests for debugging
        echo ""
        echo "Repository status:"
        "${KOPIA[@]}" repository status 2>&1 || true
    fi
    
    echo "=== End Discovery Mode ==="
    echo ""
}

function select_snapshot_to_restore {
    echo "Selecting snapshot to restore" >&2
    
    # List snapshots for the specific data directory path
    # This ensures we get snapshots for the correct username@hostname:/path
    local snapshot_list_cmd=("${KOPIA[@]}" snapshot list "${DATA_DIR}" --json)
    
    # Get the base offset from KOPIA_PREVIOUS parameter (defaults to 0)
    local -i previous_offset=${KOPIA_PREVIOUS-0}
    
    # List snapshots and find the appropriate one
    # Capture both stdout and stderr to handle cases where no snapshots exist
    local snapshot_output
    local snapshot_stderr
    snapshot_output=$("${snapshot_list_cmd[@]}" 2>&1) || true
    
    # Check if the output indicates no snapshots found
    if [[ "${snapshot_output}" == "[]" ]] || [[ "${snapshot_output}" == "" ]] || 
       [[ "${snapshot_output}" =~ "unable to find snapshots" ]] || 
       [[ "${snapshot_output}" =~ "no snapshot manifests found" ]]; then
        # No snapshots found for this identity
        return 0
    fi
    
    # Parse the JSON output
    if [[ -n "${KOPIA_RESTORE_AS_OF}" ]]; then
        echo "Restoring as of: ${KOPIA_RESTORE_AS_OF}" >&2
        # For restore-as-of, we need to filter by time first, then apply offset
        local filtered_snapshots
        filtered_snapshots=$(echo "${snapshot_output}" | jq -r "[.[] | select(.startTime <= \"${KOPIA_RESTORE_AS_OF}\")] | .[${previous_offset}:${previous_offset}+1][] | .id" 2>/dev/null || true)
        echo "${filtered_snapshots}"
    elif [[ -n "${KOPIA_SHALLOW}" ]]; then
        echo "Shallow restore, showing last ${KOPIA_SHALLOW} snapshots with previous offset ${previous_offset}" >&2
        # Apply previous offset within the shallow limit
        echo "${snapshot_output}" | jq -r ".[${previous_offset}:${KOPIA_SHALLOW}+${previous_offset}][] | .id" 2>/dev/null | head -1 || true
    else
        # Get snapshot with offset (0 = latest, 1 = previous, etc.)
        echo "Using previous offset: ${previous_offset} (0=latest, 1=previous, etc.)" >&2
        echo "${snapshot_output}" | jq -r ".[${previous_offset}].id" 2>/dev/null || true
    fi
}

function do_restore {
    echo "=== Starting restore ==="
    
    # Check if discovery mode is enabled
    if [[ "${KOPIA_DISCOVER_SNAPSHOTS}" == "true" ]]; then
        echo "Discovery mode enabled - will list available snapshots if restore fails"
    fi
    
    # Select snapshot to restore
    local snapshot_id
    snapshot_id=$(select_snapshot_to_restore)
    
    if [[ -z ${snapshot_id} ]]; then
        echo "No eligible snapshots found"
        
        # If discovery mode is enabled and no snapshots found, list available snapshots
        if [[ "${KOPIA_DISCOVER_SNAPSHOTS}" == "true" ]]; then
            echo "No snapshots found for ${KOPIA_OVERRIDE_USERNAME:-$(whoami)}@${KOPIA_OVERRIDE_HOSTNAME:-$(hostname)}:${DATA_DIR}"
            discover_available_snapshots
        fi
        
        echo "=== No data will be restored ==="
        return 0
    fi
    
    echo "Selected snapshot with id: ${snapshot_id}"
    
    # Restore the snapshot with proper error handling
    # Note: We restore directly to ${DATA_DIR} instead of changing directory first
    # to avoid issues with path construction in Kopia
    # Use --write-files-atomically to avoid temporary .kopia-entry files in wrong locations
    if ! "${KOPIA[@]}" snapshot restore "${snapshot_id}" "${DATA_DIR}" \
        --write-files-atomically \
        --ignore-permission-errors; then
        
        # If discovery mode is enabled and restore failed, show available snapshots
        if [[ "${KOPIA_DISCOVER_SNAPSHOTS}" == "true" ]]; then
            echo "Failed to restore snapshot: ${snapshot_id}"
            discover_available_snapshots
        fi
        
        error 1 "Failed to restore snapshot: ${snapshot_id}"
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