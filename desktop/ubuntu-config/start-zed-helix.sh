#!/bin/bash
#
# start-zed-helix.sh - Zed startup script for Ubuntu GNOME desktop
#
# This is a thin wrapper that defines GNOME-specific terminal launching
# and sources the shared core startup logic.

# =========================================
# Ubuntu GNOME-specific configuration
# =========================================

HELIX_DESKTOP_NAME="Ubuntu GNOME"

# =========================================
# Terminal launcher (Ubuntu uses gnome-terminal)
# =========================================

launch_terminal() {
    local title="$1"
    local working_dir="$2"
    shift 2
    # Remaining args are the command
    # The script itself has a trap to keep terminal open on exit
    gnome-terminal --title="$title" --geometry=120x40 --working-directory="$working_dir" -- "$@" &
}

# =========================================
# Source shared core and run
# =========================================

# Find and source the shared core script
CORE_SCRIPT="/usr/local/bin/start-zed-core.sh"
if [ ! -f "$CORE_SCRIPT" ]; then
    CORE_SCRIPT="/helix-dev/shared/start-zed-core.sh"
fi
if [ ! -f "$CORE_SCRIPT" ]; then
    echo "ERROR: start-zed-core.sh not found!"
    exit 1
fi

source "$CORE_SCRIPT"
start_zed_helix
