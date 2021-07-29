#! /bin/bash

set -e -o pipefail

function do_shutdown {
    rc="$1"

    echo "Initiating shutdown. Exit code: $rc"

    # /tmp/exit_code is read by main process when sshd terminates and is used as
    # the return code for the container.
    echo "$rc" >> /tmp/exit_code

    PIDFILE="/run/sshd.pid"
    [[ -e "$PIDFILE" ]] && kill -SIGTERM "$(<"$PIDFILE")"
}

function do_rsync {
    # rsync changes are restricted to the /data directory of the container
    LANG=C rrsync /data
}

#-- These are the only commands allowed to be executed by the source side:
# Source can initiate an rsync
if [[ "$SSH_ORIGINAL_COMMAND" =~ ^rsync( ) ]]; then
    do_rsync
# Source can tell us (destination) to shutdown & pass a numeric result code
elif [[ "$SSH_ORIGINAL_COMMAND" =~ ^shutdown( )+([0-9]+)$ ]]; then
    do_shutdown "${BASH_REMATCH[2]}"
# Everything else is an error
else
    echo "Invalid command: $SSH_ORIGINAL_COMMAND"
    exit 1
fi
