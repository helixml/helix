#!/bin/bash
# start-vscode-helix.sh - VS Code + Roo Code startup for Ubuntu GNOME (sources shared core)

HELIX_DESKTOP_NAME="Ubuntu GNOME (VS Code)"

launch_terminal() {
    local title="$1" working_dir="$2"; shift 2
    ghostty --gtk-single-instance=false --title="$title" --working-directory="$working_dir" -e "$@" &
}

for p in /usr/local/bin /helix-dev/shared; do
    [ -f "$p/start-zed-core.sh" ] && source "$p/start-zed-core.sh" && start_vscode_helix && exit 0
done
echo "ERROR: start-zed-core.sh not found!" && exit 1
