#! /bin/bash

set -e -o pipefail

echo "Scribe restic container version: ${version:-unknown}"
echo  "$@"


# Force the associated backup host name to be "scribe"
RESTIC_HOST="scribe"
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

# Ensure the repo has been initialized
function ensure_initialized {
    echo "== Initialize Dir ======="
    # Try a restic command and capture the rc & output
    outfile=$(mktemp -q)
    if ! restic snapshots 2>"$outfile"; then
        output=$(<"$outfile")
        # Match against error string for uninitialized repo
        if [[ $output =~ .*(Is there a repository at the following location).* ]]; then
            restic init
        else
            error 3 "failure checking existence of repository"
        fi
    fi
    rm -f "$outfile"
}

function do_backup {
    echo "=== Starting backup ==="
    pushd "${DATA_DIR}"
    restic backup --host "${RESTIC_HOST}" .
    popd
}

function do_forget {
    echo "=== Starting forget ==="
    if [[ -n ${FORGET_OPTIONS} ]]; then
        #shellcheck disable=SC2086
        restic forget --host "${RESTIC_HOST}" ${FORGET_OPTIONS}
    fi
}

function do_prune {
    echo "=== Starting prune ==="
    restic prune
}

function do_restore {
    echo "=== Starting restore ==="
    pushd "${DATA_DIR}"
    restic restore -t . --host "${RESTIC_HOST}" latest
    popd
}
echo "Testing mandatory env variables"
# Check the mandatory env variables
for var in RESTIC_CACHE_DIR \
           RESTIC_PASSWORD \
           RESTIC_REPOSITORY \
           DATA_DIR \
           ; do
    check_var_defined $var
done

for op in "$@"; do
    case $op in
        "backup")
            ensure_initialized
            do_backup
            do_forget
            ;;
        "prune")
            do_prune
            ;;
        "restore")
            do_restore
            ;;
        *)
            error 2 "unknown operation: $op"
            ;;
    esac
done

echo "=== Done ==="
