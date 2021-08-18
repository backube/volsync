#! /bin/bash
RESTORE_AS_OF="2021-08-02 15:00:00.578982463 +0000 UTC m=+170.978358482"
SELECT_PREVIOUS="0"

DATE_FROM_TS=`cut -d'.' <<< $RESTORE_AS_OF -f1`
DATE_AS_EPOCH=$(date --date="$DATE_FROM_TS" +%s)

# echo "RESTORE_AS_OF: $RESTORE_AS_OF"
# echo "SELECT_PREVIOUS: $SELECT_PREVIOUS"
# echo "DATE_FROM_TS: $DATE_FROM_TS"
# echo "DATE_AS_EPOCH: $DATE_AS_EPOCH"

#######################################
# Trims the provided timestamp and 
# returns one in the format: YYYY-MM-DD hh:mm:ss
# Globals:
#   None
# Arguments
#   Timestamp in format YYYY-MM-DD HH:mm:ss.ns
#######################################
function trim_timestamp() {
    local trimmed_timestamp=$(cut -d'.' <<< "$1" -f1)
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
    # printf "Received timestamp: %s\n" "${timestamp}"
    local trimmed_timestamp=$(trim_timestamp "${timestamp}")
    # printf "Trimmed timestamp: %s\n" "${trimmed_timestamp}"
    local date_as_epoch=$(date --date="${trimmed_timestamp}" +%s)
    # printf "date_as_epoch: %s\n" "${date_as_epoch}"
    # echo "internally: $date_as_epoch"
    echo "${date_as_epoch}"
}

#######################################
# Retrieves the 
#######################################
function restic_timestamps_as_epoch() {
    local timestamps=$(restic -r ${RESTIC_REPOSITORY} snapshots | grep /data | awk '{print $2 " " $3}')
    # printf "timestamps: %s " "${timestamps}"
    for var in ${timestamps}; do
        echo `get_epoch_from_timestamp $var`
    done
}


#######################################
# Prints array 
# Globals:
#   None
# Arguments:
#   Name of array
#######################################
function print_array() {
    local name=$1[@]
    local _arr=("${!name}")
    for i in "${_arr[@]}"; do
        echo "$i"
    done 
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
    local -i right=$(( ${#_arr[@]} - 1 ))

    while (( ${left} < ${right} )); do
        # triangle swap 
        local -i temp="${_arr[$left]}"
        _arr[$left]="${_arr[$right]}"
        _arr[$right]="$temp"

        # increment indices 
        ((left++))
        ((right--))
    done
}

#######################################
# Selects the earliest Restic snapshot to restore from 
# that was created before RESTORE_AS_OF. If SELECT_PREVIOUS
# is non-zero, then it selects the n-th snapshot older than RESTORE_AS_OF
# Globals:
#   SELECT_PREVIOUS
#   RESTORE_AS_OF
# Arguments:
#   None
#######################################
function select_restic_snapshot_to_restore() {
    local offset=${SELECT_PREVIOUS}
    local target_epoch=$(get_epoch_from_timestamp "${RESTORE_AS_OF}")
    # list of epochs 
    declare -a epochs
    # create an associative array that maps numeric epoch to the restic snapshot IDs 
    declare -A epochs_to_snapshots

    # go through the timestamps received from restic
    IFS=$'\n'; for line in $(cat mover-restic/restic_output | grep /data | awk '{print $1 "\t" $2 " " $3}'); do
        # extract the proper variables 
        local snapshot_id=$(echo -e ${line} | cut -d$'\t' -f1)
        local snapshot_ts=$(echo -e ${line} | cut -d$'\t' -f2)
        local snapshot_epoch=$(get_epoch_from_timestamp ${snapshot_ts})
        epochs+=("${snapshot_epoch}")
        epochs_to_snapshots[${snapshot_epoch}]="${snapshot_id}"
    done

    # reverse the list so the first element has the most recent timestamp
    reverse_array epochs 
    local -i idx=0
    # find the first epoch in the list less than or equal to RESTORE_AS_OF
    while (( ${target_epoch} < ${epochs[${idx}]} )); do 
        local -i nextIdx=${idx}+1
        # if we reached the end of the list 
        if (( ${nextIdx} == ${#epochs[@]} )); then 
            break
        fi
        (( idx++ ))
    done

    # get the epoch + the offset, or the oldest epoch
    local -i lastIdx=$(( ${#epochs[@]} - 1 ))
    local -i offsetIdx=$(( ${idx} + ${offset} ))
    local selectedEpoch=${epochs[${lastIdx}]}
    if (( ${offsetIdx} <= ${lastIdx} )); then
        selectedEpoch=${epochs[${offsetIdx}]}
    fi
    local selectedId=${epochs_to_snapshots[${selectedEpoch}]}
    echo "${selectedId}"
}


# epoch_ts=$(get_epoch_from_timestamp $RESTORE_AS_OF)
# echo "epoch_ts: $epoch_ts"

# echo `restic_timestamps_as_epoch`

select_restic_snapshot_to_restore