#!/usr/bin/env bash
# GOW init script: Configure device permissions

set -e

gow_log "**** Configure devices ****"

gow_log "Exec device groups"
# Make sure we're in the right groups to use all the required devices
# We're actually relying on word splitting for this call, so disable the
# warning from shellcheck
# shellcheck disable=SC2086
/opt/gow/ensure-groups ${GOW_REQUIRED_DEVICES:-/dev/uinput /dev/input/event*}

gow_log "DONE"
