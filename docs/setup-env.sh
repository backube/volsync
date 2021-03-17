#!/usr/bin/env bash

SCRIPT_DIR="$(dirname "$(realpath "$0")")"
VENV_NAME="$SCRIPT_DIR/../.venv"


python3 -m venv "$VENV_NAME"

# shellcheck disable=SC1090
source "$VENV_NAME/bin/activate"

pip install --upgrade pip
pip install --upgrade -r requirements.txt
