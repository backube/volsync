#!/bin/bash

##################################################################
# Wait until all of the given list of
# ReplicationSource objects have
# non-null fields for .status.syncthing.address
# and .status.syncthing.ID
# Globals:
#   (string) NAMESPACE
# Arguments:
#   (string) ReplicationSource: name of a ReplicationSource object
# Returns:
#   None
##################################################################
wait_for_syncthing_ready() {
	replication_source="${1}"

	echo "waiting for syncthing address and ID for ${replication_source}..."
	# sleep until replicationsource has syncthing address and ID

	st_address=""
	st_device_id=""
	while [[ -z "${st_address}" || -z "${st_device_id}" ]]; do
		st_address=$(kubectl get replicationsource "${replication_source}" -n "${NAMESPACE}" -o jsonpath='{.status.syncthing.address}')
		st_device_id=$(kubectl get replicationsource "${replication_source}" -n "${NAMESPACE}" -o jsonpath='{.status.syncthing.ID}')
		if [[ -z "${st_address}" || -z "${st_device_id}" ]]; then
			echo "syncthing info not yet available, current status:"
			kubectl get replicationsource "${replication_source}" -n "${NAMESPACE}" -o jsonpath='{.status}'
			sleep 5
		fi
	done
}

##########################################################
# Connects the source RS to the target RS and sets the
# target RS as a Syncthing introducer, if specified.
# Globals:
#   (string) NAMESPACE
# Arguments:
#   (string) source_rs_name: name of the ReplicationSource
#   (string) target_rs_name: name of the ReplicationSource
#   (int)    as_introducer: 1 if the target RS should be
#                           set as an introducer,
#                           0 otherwise
# Returns:
#   (int) 0 on success, 1 on failure
##########################################################
connect_to_target() {
	source_rs_name="${1}"
	target_rs_name="${2}"
	as_introducer="${3}"

	# grab the target RS's address and device ID
	target_address=$(kubectl get replicationsource "${target_rs_name}" -n "${NAMESPACE}" -o jsonpath='{.status.syncthing.address}')
	target_device_id=$(kubectl get replicationsource "${target_rs_name}" -n "${NAMESPACE}" -o jsonpath='{.status.syncthing.ID}')


	# grab the current source RS's peer list as a json list and append the target RS's address and device ID
	current_spec=$(kubectl get replicationsource "${source_rs_name}" -n "${NAMESPACE}" -o jsonpath='{.spec}' | jq -c '{spec: .}')

	# insert new_peers into .spec.syncthing.peers in skeleton_spec
	new_spec=""
	if [[ "${as_introducer}" -eq '1' ]]; then
		# introducer
		new_spec=$(echo "${current_spec}" | jq -c \
			--arg deviceID "${target_device_id}" \
			--arg address "${target_address}" \
			'.spec.syncthing.peers |= . + [{"ID": $deviceID, "address": $address, "introducer": true}]'
		)
	else
			# not introducer
			new_spec=$(echo "${current_spec}" | jq -c \
			--arg deviceID "${target_device_id}" \
			--arg address "${target_address}" \
			'.spec.syncthing.peers |= . + [{"ID": $deviceID, "address": $address, "introducer": false}]'
		)
	fi

	# patch the replicationsource with the new spec
	kubectl patch replicationsource "${source_rs_name}" -n "${NAMESPACE}" --type=merge --patch "${new_spec}"
}


############################################################
# Connects two Syncthing-based ReplicationSources
# Globals:
#   (string) NAMESPACE
# Arguments:
#   (string) source_rs: name of the source ReplicationSource
#   (string) target_rs: name of the target ReplicationSource
# Returns:
#   1 if the connection failed, 0 otherwise
############################################################
connect_syncthing() {
	source_rs="${1}"
	target_rs="${2}"
	
	if ! [[ $(connect_to_target "${source_rs}" "${target_rs}" "0") ]]; then
		return 1
	fi

	if ! [[ $(connect_to_target "${target_rs}" "${source_rs}" "0") ]]; then
		return 1
	fi
	return 0
}


##################################################################
# Ensures all ReplicationSources
# have each other in their peer list
# Arguments:
#   None
# Globals:
#   NAMESPACE
# Returns:
#  0 if all ReplicationSources have each other in their peer list, 
#  1 otherwise
##################################################################
verify_rs_are_connected() {
	# get all of the IDs for every replicationsource
	replication_sources=$(update_replication_sources)
	st_ids=$(echo "${replication_sources}" | jq -r '.[] | .status.syncthing.ID')
	for st_id in ${st_ids}; do
		rs=$(echo "${replication_sources}" | jq -r --arg currentID "${st_id}" \
			'.[] | select(.status.syncthing.ID == $currentID)'
		)

		# get the peer list for the current replicationsource
		peers=$(echo "${rs}" | jq -r '.spec.syncthing.peers')
		peer_ids=$(echo "${peers}" | jq -r '.[] | .ID')

		# all syncthing IDs in cluster
		full_peer_ids=$(echo "${replication_sources}" | jq -r \
			--arg currentID "${st_id}" \
			'.[] | select(.status.syncthing.ID != $currentID) | .status.syncthing.ID'
		)

		# ensure that each element of full_peer_ids is in peer_ids
		for peer_id in ${full_peer_ids}; do
			if [[ ! "${peer_ids}" =~ ${peer_id} ]]; then
				# ReplicationSource ${st_id} does not have ${peer_id} in its peer list
				return 1
			fi
		done
	done
	return 0
}


########################################################################
# Ensures that all ReplicationSources
# are connected to the Syncthing
# devices listed in their spec
# Globals:
#   (json) replication_sources
# Arguments:
#   (string) replicationsource name
# Returns:
#  0 if all ReplicationSources are connected to their Syncthing devices,
#  1 otherwise
########################################################################
all_peers_are_connected() {
	rs_name="${1}"

	# get the record for the given replicationsource
	replication_sources=$(update_replication_sources)
	rs=$(echo "${replication_sources}" | jq --arg rsName "${rs_name}" '.[] | select(.metadata.name == $rsName)')
	peer_ids=$(echo "${rs}" | jq -r '.spec.syncthing.peers[].ID')

	# check if .status.syncthing.peers is null or empty
	if [[ $(echo "${rs}" | jq '.status.syncthing | has("peers")') == "true" ]]; then
		connected_peers=$(echo "${rs}" | jq -r '.status.syncthing.peers')
		connected_peer_ids=$(echo "${connected_peers}" | jq -r '.[] | .ID')
		IFS=$'\n' read -rd '' -a connected_peer_ids <<< "${connected_peer_ids}" || true
		unset IFS

		# make sure that each peer_id exists in .status.syncthing.peers
		for peer_id in "${peer_ids[@]}"; do
			if [[ ! "${connected_peer_ids[*]}" =~ ${peer_id} ]]; then
				# ReplicationSource ${rs_name} does not have ${peer_id} in its peer list
				return 1
			fi

			# grab the peer_id and check if connected: true
			peer=$(echo "${connected_peers}" | jq -r --arg peerID "${peer_id}" \
				'.[] | select(.ID == $peerID)')
			if [[ $(echo "${peer}" | jq '.connected') != "true" ]]; then
				# ${peer_id} is not connected in ${rs_name}
				return 1
			fi
		done
	else
		# ReplicationSource ${rs_name} does not have any peers
		return 1
	fi

	return 0
}

######################################
# Update the replication_sources
# Globals:
#  (string) NAMESPACE
# Returns:
#  (json) replication_sources
######################################
update_replication_sources() {
	sources=$(kubectl get replicationsource -n "${NAMESPACE}" -o json | jq '.items')
	echo "${sources}"
}
