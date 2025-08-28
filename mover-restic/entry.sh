#! /bin/bash
# sleep forever
# sleep 9999999999

echo "Starting container"

set -e -o pipefail

echo "VolSync restic container version: ${version:-unknown}"
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

if [[ "${SSH_KEYS}" == "true" ]]; then
  echo "Using SSH Keys."
  chmod 711 ~/.ssh
  
  cat - <<SSHCONFIG > ~/.ssh/config
Host *
  # Wait max 30s to establish connection
  ConnectTimeout 30
  # Use the identity provided via attached Secret
  IdentityFile /keys/key
  # Enable protocol-level keepalive to detect connection failure
  ServerAliveCountMax 4
  ServerAliveInterval 30
  # We don't know the key of the server, so be strict
  StrictHostKeyChecking no
  # Using protocol-level, so we don't need TCP-level
  TCPKeepAlive no
SSHCONFIG
fi

declare -a RESTIC
RESTIC=("restic")
if [[ -n "${CUSTOM_CA}" ]]; then
    echo "Using custom CA."
    RESTIC+=(--cacert "${CUSTOM_CA}")
fi

"${RESTIC[@]}" version

# Force the associated backup host name to be "volsync"
RESTIC_HOST="volsync"
# Make restic output progress reports every 10s
export RESTIC_PROGRESS_FPS=0.1

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

function check_contents {
    echo "== Checking directory for content ==="
    DIR_CONTENTS="$(ls -A "${DATA_DIR}" --ignore="lost+found")"
    if [ -z "${DIR_CONTENTS}" ]; then
        echo "== Directory is empty skipping backup ==="
        exit 0
    fi
}

# Ensure the repo has been initialized
function ensure_initialized {
    echo "=== Check for dir initialized ==="
    # check for restic config and capture rc
    # See: https://restic.readthedocs.io/en/stable/075_scripting.html#check-if-a-repository-is-already-initialized
    set +e  # Don't exit on command failure

    outfile=$(mktemp -q)
    timeout 10s "${RESTIC[@]}" cat config > /dev/null 2>"$outfile"
    rc=$?

    set -e  # Exit on command failure

    case $rc in
    0)
        echo "dir is initialized"
        ;;
    1 | 124)
        # 124 will be returned if timeout occurs
        # Using "timeout" above as restic commands such as cat now retry, so a missing repository will
        # mean it keeps retrying
        #
        # This can happen for some providers (e.g. minio) if the bucket does not exist
        # Restic will return 10 if the bucket exists and no restic repo at the path exists, but will
        # still return 1 if the bucket itself doesn't exist.
        # We can proceed with trying an init which will create the bucket (and path in the bucket if there is one)
        # restic init should fail if somehow the repo already exists when init is run or if it's unable to
        # create the bucket
        output=$(<"$outfile")
        # Match against error string for uninitialized repo
        # This string also appears when credentials are incorrect (in which case
        # the following cmd `restic init` will also fail)
        if [[ $output =~ .*(Is there a repository at the following location).* ]]; then
            echo "=== Initialize Dir ==="
            "${RESTIC[@]}" init
        else
            cat "$outfile"
            error 3 "failure checking existence of repository"
        fi
        ;;
    10)
        # rc = 10  Repository does not exist (since restic 0.17.0)
        echo "=== Initialize Dir ==="
        "${RESTIC[@]}" init
        ;;
    11)
        # rc = 11  Failed to lock repository (since restic 0.17.0)
        cat "$outfile"
        error 3 "failure locking repository"
        ;;
    12)
        # rc = 12 Wrong password (since restic 0.17.1)
        cat "$outfile"
        error 3 "failure connecting to repository, incorrect password"
        ;;
    *)
        cat "$outfile"
        error 3 "failure checking existence of repository"
        ;;
    esac

    rm -f "$outfile"
}

function do_backup {
    echo "=== Starting backup ==="
    pushd "${DATA_DIR}"
    "${RESTIC[@]}" backup --host "${RESTIC_HOST}" --exclude='lost+found' .
    popd
}

function do_forget {
    echo "=== Starting forget ==="
    if [[ -n ${FORGET_OPTIONS} ]]; then
        #shellcheck disable=SC2086
        "${RESTIC[@]}" forget --host "${RESTIC_HOST}" ${FORGET_OPTIONS}
    fi
}

function do_unlock {
    echo "=== Starting unlock ==="
    # Try a restic unlock and capture the rc & output
    outfile=$(mktemp -q)
    #if ! "${RESTIC[@]}" unlock --remove-all 2>"$outfile"; then
    if ! "${RESTIC[@]}" unlock 2>"$outfile"; then
        output=$(<"$outfile")
        # Match against error string for uninitialized repo
        if [[ $output =~ .*(Is there a repository at the following location).* ]]; then
            # No repo, no need to unlock
            cat "$outfile"
            echo "No repo, ignoring unlock error"
        else
            cat "$outfile"
            error 3 "failure unlocking repository"
        fi
    fi
    rm -f "$outfile"
}

function do_prune {
    echo "=== Starting prune ==="
    "${RESTIC[@]}" prune
}

#######################################
# Trims the provided timestamp and
# returns one in the format: YYYY-MM-DD hh:mm:ss
# Globals:
#   None
# Arguments
#   Timestamp in format YYYY-MM-DD HH:mm:ss.ns
#######################################
function trim_timestamp() {
    local trimmed_timestamp
    trimmed_timestamp=$(cut -d'.' -f1 <<<"$1")
    echo "${trimmed_timestamp}"
}

#######################################
# Provides the UNIX Epoch for the given timestamp
# Globals:
#   None
# Arguments:
#   timestamp
#######################################
function get_epoch_from_timestamp() {
    local timestamp="$1"
    local date_as_epoch
    date_as_epoch=$(date --date="${timestamp}" +%s)
    echo "${date_as_epoch}"
}


#######################################
# Reverses the elements within an array,
# inspired by: https://unix.stackexchange.com/a/412870
# Globals:
#   None
# Arguments:
#   Name of array
#######################################
function reverse_array() {
    local -n _arr=$1

    # set indices
    local -i left=0
    local -i right=$((${#_arr[@]} - 1))

    while ((left < right)); do
        # triangle swap
        local -i temp="${_arr[$left]}"
        _arr[left]="${_arr[$right]}"
        _arr[right]="$temp"

        # increment indices
        ((left++))
        ((right--))
    done
}

################################################################
# Selects the first restic snapshot available for the
# given constraints. If RESTORE_AS_OF is defined, then
# only snapshots that were created prior to it are considered.
# If SELECT_PREVIOUS is defined, then the n-th snapshot
# is selected under the matching criteria.
# If a snapshot satisfying the conditions is found, then its ID
# is returned.
#
# Globals:
#   SELECT_PREVIOUS
#   RESTORE_AS_OF
# Arguments:
#   None
################################################################
function select_restic_snapshot_to_restore() {
    # list of epochs
    declare -a epochs
    # create an associative array that maps numeric epoch to the restic snapshot IDs
    declare -A epochs_to_snapshots

    local restic_snapshots
    if ! restic_snapshots=$("${RESTIC[@]}" -r "${RESTIC_REPOSITORY}" snapshots); then
      error 3 "failure getting list of snapshots from repository"
    fi

    # declare vars to be used in the loop
    local snapshot_id
    local snapshot_ts
    local trimmed_timestamp
    local snapshot_epoch

    # go through the timestamps received from restic
    IFS=$'\n'
    for line in $(echo -e "${restic_snapshots}" | grep /data | awk '{print $1 "\t" $2 " " $3}'); do
        # extract the proper variables
        snapshot_id=$(echo -e "${line}" | cut -d$'\t' -f1)
        snapshot_ts=$(echo -e "${line}" | cut -d$'\t' -f2)
        trimmed_timestamp=$(trim_timestamp "${snapshot_ts}")
        snapshot_epoch=$(get_epoch_from_timestamp "${trimmed_timestamp}")
        epochs+=("${snapshot_epoch}")
        epochs_to_snapshots[${snapshot_epoch}]="${snapshot_id}"
    done

    # reverse sorted epochs so the most recent epoch is first
    reverse_array epochs

    local -i offset=0
    local target_epoch
    if [[ -n ${RESTORE_AS_OF} ]]; then
        # move to the position of the satisfying epoch
        target_epoch=$(get_epoch_from_timestamp "${RESTORE_AS_OF}")
        while ((offset < ${#epochs[@]})); do
            if ((${epochs[${offset}]} <= target_epoch)); then
                break
            fi
            ((offset++))
        done
    fi

    # offset the epoch selection if SELECT_PREVIOUS is defined
    local -i select_offset=${SELECT_PREVIOUS-0}
    ((offset+=select_offset))

    # if there is a snapshot matching the provided parameters
    if (( offset < ${#epochs[@]} )); then
        local selected_epoch=${epochs[${offset}]}
        local selected_id=${epochs_to_snapshots[${selected_epoch}]}
        echo "${selected_id}"
    fi
}


#######################################
# Restores from a selected snapshot if
# RESTORE_AS_OF is provided, otherwise
# restores from the latest restic snapshot
# Globals:
#   RESTORE_AS_OF
#   DATA_DIR
#   RESTIC_HOST
# Arguments:
#   None
#######################################
function do_restore {
    echo "=== Starting restore ==="
    # restore from specific snapshot specified by timestamp, or latest
    local snapshot_id
    snapshot_id=$(select_restic_snapshot_to_restore)
    if [[ -z ${snapshot_id} ]]; then
        echo "No eligible snapshots found"
        echo "=== No data will be restored ==="
    else
        if [[ -n ${RESTORE_OPTIONS} ]]; then
          echo "RESTORE_OPTIONS: ${RESTORE_OPTIONS}"
        fi
        pushd "${DATA_DIR}"
        echo "Selected restic snapshot with id: ${snapshot_id}"
        # Running this cmd can be finicky with spaces, do not put quotes around ${RESTORE_OPTIONS}
        #shellcheck disable=SC2086
        "${RESTIC[@]}" restore "${snapshot_id}" -t . --host "${RESTIC_HOST}" --include-xattr "user.*" ${RESTORE_OPTIONS}
        popd
    fi
}

echo "Testing mandatory env variables"
# Check the mandatory env variables
for var in PRIVILEGED_MOVER \
           RESTIC_CACHE_DIR \
           RESTIC_PASSWORD \
           RESTIC_REPOSITORY \
           DATA_DIR \
           ; do
    check_var_defined $var
done
START_TIME=$SECONDS
for op in "$@"; do
    case $op in
        "unlock")
            do_unlock
            ;;
        "backup")
            check_contents
            ensure_initialized
            do_backup
            do_forget
            ;;
        "prune")
            do_prune
            ;;
        "restore")
            ensure_initialized
            do_restore
            sync -f "${DATA_DIR}"
            ;;
        *)
            error 2 "unknown operation: $op"
            ;;
    esac
done
echo "Restic completed in $(( SECONDS - START_TIME ))s"
echo "=== Done ==="
# sleep forever so that the containers logs can be inspected
# sleep 9999999
