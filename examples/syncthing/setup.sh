#!/bin/sh
#
# Configure running syncthing address with a list of devices provided

###########################
# Logs the given input
# Globals:
#   None 
# Arguments:
#   String(s) to be logged
# Returns:
#   Formatted log message
##########################
log_msg() {
  msg="$*"
  echo "===== ${msg} ====="
}


#############################################
# Add a new device to Syncthing 
# Globals:
#   None
# Arguments:
#   (string) Name of file to read from
# Returns:
#   (string) Updated config JSON 
#############################################
configure_syncthing() {
  # todo: go through list of data

  # process input 
  input_file="$1"
  url=$(jq -r '.url' < "${input_file}")
  apikey=$(jq  -r '.apikey' < "${input_file}")

  # create an array of devices
  devices='{"devices": [], "folderSharedWith": []}'
  for d in $(jq -r '.devices[]' -c < "${input_file}"); do
    device_id=$(echo "${d}" | jq -r '.deviceID')
    device_address=$(echo "${d}" | jq -r '.deviceAddress')
    device_name=$(echo "${d}" | jq -r '.deviceName')

    # create device entry
    device_entry=$(jq -n \
      --arg deviceID "${device_id}" \
      --arg deviceName "${device_name}" \
      --arg tcpAddress "${device_address}" \
      '{deviceID: $deviceID,name: $deviceName,addresses: [$tcpAddress],compression: "metadata",certName: "",introducer: false,skipIntroductionRemovals: false,introducedBy: "",paused: false,allowedNetworks: [],autoAcceptFolders: false,maxSendKbps: 0,maxRecvKbps: 0,ignoredFolders: [],maxRequestKiB: 0,untrusted: false,remoteGUIPort: 0}'
    )
    devices=$(echo "${devices}" | jq ".devices += [${device_entry}]")

    # add the devices to the list of devices that the folder is shared with
    folder_device_entry=$(jq -n \
      --arg deviceID "${device_id}" \
      '{deviceID: $deviceID, introducedBy: "", encryptionPassword: ""}'
    )
    devices=$(echo "${devices}" | jq ".folderSharedWith += [${folder_device_entry}]")
  done

  # extract the device lists
  new_devices=$(echo "${devices}" | jq -c '.devices')
  devices_to_share_with=$(echo "${devices}" | jq -c '.folderSharedWith')

  # update the remote config with our new lists
  config=$(curl -X GET -H "X-API-Key: ${apikey}" --insecure --silent "${url}/rest/config")
  new_config=$(echo "${config}" | jq ".devices += ${new_devices}" | jq ".folders[0].devices += ${devices_to_share_with}")

  # update syncthing using the new config 
  curl -X PUT -H "X-API-Key: ${apikey}" -d "${new_config}" --insecure "${url}/rest/config"
}

configure_syncthing "$1"
