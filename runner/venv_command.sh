#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

export APP_FOLDER=${APP_FOLDER:=""}

# a wrapper script to first activate a venv in a folder
# before running the provided command
if [[ -z "$APP_FOLDER" ]]; then
  echo >&2 "APP_FOLDER variable is missing"
  exit 1
fi

if [[ ! -d "$APP_FOLDER" ]]; then
  echo >&2 "$APP_FOLDER is not a folder"
  exit 1
fi

if [[ ! -d "$APP_FOLDER/venv" ]]; then
  echo >&2 "$APP_FOLDER/venv does not have a venv"
  exit 1
fi

cd $APP_FOLDER
. ./venv/bin/activate
eval "$@"
