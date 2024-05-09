#!/bin/bash
#
# Configure and run Syncthing
# Author(s): The VolSync Authors
# License: AGPL v3

set -e -o pipefail


#####################################################
# Logs the given input
# Globals:
#   None
# Arguments:
#   String(s) to be logged
# Returns:
#   Formatted log message
#####################################################
log_msg() {
  local msg="$*"
  echo "===== ${msg} ====="
}

log_msg "STARTING CONTAINER"
log_msg "VolSync Syncthing container version: ${version:-unknown}"
log_msg "${@}"

SCRIPT_FULLPATH="$(realpath "$0")"
SCRIPT="$(basename "$SCRIPT_FULLPATH")"
SCRIPT_DIR="$(dirname "$SCRIPT_FULLPATH")"

# Do not do this debug mover code if this is already the
# mover script copy in /tmp
if [[ $DEBUG_MOVER -eq 1 && "$SCRIPT_DIR" != "/tmp" ]]; then
  MOVER_SCRIPT_COPY="/tmp/$SCRIPT"
  cp $SCRIPT_FULLPATH $MOVER_SCRIPT_COPY

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
  echo "$MOVER_SCRIPT_COPY $@"
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

# variables we can't proceed without
required_vars=(
  SYNCTHING_DATA_DIR
  SYNCTHING_CERT_DIR
  SYNCTHING_CONFIG_DIR
  STGUIAPIKEY
  PRIVILEGED_MOVER
)

#####################################################
# Error and exit if a variable isn't defined
# check_var_defined "MY_VAR"
# Globals:
#   None
# Arguments:
#   String - variable to check
# Returns:
#   None
#####################################################
check_var_defined() {
    if [[ -z ${!1} ]]; then
        error 1 "${1} must be defined"
    fi
}

#####################################################
# Replaces the placeholders in config.xml
# with the values of their respective envs
# Arguments:
#   Filepath - path to the config file to operate on
# Globals:
#   SYNCTHING_DATA_DIR
#   SYNCTHING_DATA_TRANSFERMODE
# Returns:
#   None
#####################################################
preconfigure_folder() {
  # HACK: make the config.xml template more configurable
  #       in case these variables change

  local filepath="${1}"
  # use a vertical bar here so sed doesn't misinterpret the path
  sed -i "s|SYNCTHING_DATA_DIR|${SYNCTHING_DATA_DIR}|g" "${filepath}"
  sed -i "s/SYNCTHING_DATA_TRANSFERMODE/${SYNCTHING_DATA_TRANSFERMODE}/g" "${filepath}"
}

#####################################################
# Generates the server certificate in the config
# directory but only if the cert does not exist.
# Arguments:
# 	None
# Globals:
# 	SYNCTHING_CONFIG_DIR
# Returns:
# 	None
#####################################################
ensure_server_certificates() {
  if ! [[ -f "${SYNCTHING_CONFIG_DIR}/cert.pem" ]]; then
    # use openssl to generate a new server cert
    log_msg "Generating server certs in ${SYNCTHING_CONFIG_DIR}/cert.pem"
    openssl req -x509 -newkey rsa:4096 -keyout "${SYNCTHING_CONFIG_DIR}/key.pem" -out "${SYNCTHING_CONFIG_DIR}/cert.pem" -sha256 -days 3650 -nodes -subj "/CN=syncthing" -addext "extendedKeyUsage = serverAuth, clientAuth"
  fi

  return 0
}

#####################################################
# Copies the HTTPS certificates from the
# predefined certificate directory
# to the config directory.
# Arguments:
# 	None
# Globals:
# 	SYNCTHING_CERT_DIR
# 	SYNCTHING_CONFIG_DIR
# Returns:
# 	None
#####################################################
ensure_https_certificates() {
  # copy the https-key.pem and https-cert.pem over to the config directory
  cp "${SYNCTHING_CERT_DIR}/https-key.pem" "${SYNCTHING_CONFIG_DIR}/https-key.pem"
  cp "${SYNCTHING_CERT_DIR}/https-cert.pem" "${SYNCTHING_CONFIG_DIR}/https-cert.pem"

  return 0
}


#####################################################
# Performs the necessary steps for
# Syncthing to run as an image
# Globals:
#   SYNCTHING_CONFIG_DIR
# Arguments:
#   None
# Returns:
#   None
#####################################################
preflight_check() {
  log_msg "Running preflight check"

  # variable definitions
  log_msg "ensuring necessary variables are defined"
  for var in "${required_vars[@]}"; do
    check_var_defined "${var}"
  done

  # populate config directory with config, if none exists
  if ! [[ -f "${SYNCTHING_CONFIG_DIR}/config.xml" ]]; then
    log_msg "populating ${SYNCTHING_CONFIG_DIR} with /config.xml"
    cp "/mover-syncthing/config-template.xml" "${SYNCTHING_CONFIG_DIR}/config.xml"
    preconfigure_folder "${SYNCTHING_CONFIG_DIR}/config.xml"
  else
    log_msg "${SYNCTHING_CONFIG_DIR}/config.xml already exists"
  fi

  # Populate data dir with our default .stignore, if none exists
  if ! [[ -f "${SYNCTHING_DATA_DIR}/.stignore" ]]; then
    log_msg "populating ${SYNCTHING_DATA_DIR} with /.stignore"
    cp "/mover-syncthing/stignore-template" "${SYNCTHING_DATA_DIR}/.stignore"
  else
    log_msg "${SYNCTHING_DATA_DIR}/.stignore already exists"
  fi

  # ensure server certificates
  ensure_server_certificates

  # ensure the HTTPS certificates
  ensure_https_certificates
}

for op in "$@"; do
  case $op in
    "run")
      # ensure our environment is configured before syncthing runs
      preflight_check

      # Use golang crypto/sha256, not minio's optimized sha256-simd
      # This ensures we can use the fips-enabled golang compiler
      # See https://github.com/syncthing/syncthing/blob/main/lib/sha256/sha256.go
      export STHASHING="standard"

      # launch syncthing
      exec syncthing -home "${SYNCTHING_CONFIG_DIR}"
      ;;
    *)
      error "unknown operation"
      ;;
  esac
done

log_msg "done"
