#!/usr/bin/env bash

set -e -o pipefail

SCRIPT_DIR="$(dirname "$(realpath "$0")")"
VENV_NAME="$SCRIPT_DIR/../.venv"


python3 -m venv "$VENV_NAME"

# shellcheck disable=SC1090,SC1091
source "$VENV_NAME/bin/activate"

cd "${SCRIPT_DIR}"
pip install --upgrade pip
pip install --upgrade -r requirements.txt
