#!/bin/bash
#
# start-zed-helix.sh - Zed startup script for Sway desktop
#
# This is a thin wrapper that defines Sway-specific terminal launching
# and sources the shared core startup logic.

# =========================================
# Sway-specific configuration
# =========================================

HELIX_DESKTOP_NAME="Sway"

# Verify WAYLAND_DISPLAY is set by Sway
if [ -z "$WAYLAND_DISPLAY" ]; then
    echo "ERROR: WAYLAND_DISPLAY not set! Sway should set this automatically."
    exit 1
fi

# =========================================
# Terminal launcher (Sway uses kitty)
# =========================================

launch_terminal() {
    local title="$1"
    local working_dir="$2"
    shift 2
    # Remaining args are the command
    # The script itself has a trap to keep terminal open on exit
    kitty --title="$title" --directory="$working_dir" "$@" &
}

# =========================================
# Pre-launch hook: Add Sway user guide if it exists
# =========================================

pre_zed_launch() {
    local USER_GUIDE_PATH="$WORK_DIR/SWAY-USER-GUIDE.md"
    if [ -f "$USER_GUIDE_PATH" ]; then
        echo "  + SWAY-USER-GUIDE.md"
        ZED_EXTRA_FILES=("$USER_GUIDE_PATH")
    fi
    echo "Using Wayland backend (WAYLAND_DISPLAY=$WAYLAND_DISPLAY)"
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
