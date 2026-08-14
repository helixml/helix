#!/bin/bash

HELIX_DESKTOP_NAME="Headless"

launch_terminal() {
    local title="$1"
    local working_dir="$2"
    shift 2
    (
        cd "$working_dir"
        "$@"
        sleep infinity
    ) >>"/tmp/helix-${title// /-}.log" 2>&1 &
}

source /usr/local/bin/start-zed-core.sh
start_zed_helix
