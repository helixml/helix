#!/bin/bash
#
# start-vscode-helix.sh - VS Code + Roo Code startup script for Ubuntu GNOME desktop
#
# This script sources start-zed-core.sh for common startup logic,
# then launches VS Code instead of Zed.

# =========================================
# Ubuntu GNOME-specific configuration
# =========================================

HELIX_DESKTOP_NAME="Ubuntu GNOME (VS Code)"

# =========================================
# Terminal launcher (Ubuntu uses ghostty)
# =========================================

launch_terminal() {
    local title="$1"
    local working_dir="$2"
    shift 2
    # Remaining args are the command
    # Ghostty options: --title, --working-directory, -e for command
    # CRITICAL: --gtk-single-instance=false prevents D-Bus activation which loses our -e args
    ghostty --gtk-single-instance=false --title="$title" --working-directory="$working_dir" -e "$@" &
}

# =========================================
# Source shared core for startup logic
# =========================================

CORE_SCRIPT="/usr/local/bin/start-zed-core.sh"
if [ ! -f "$CORE_SCRIPT" ]; then
    CORE_SCRIPT="/helix-dev/shared/start-zed-core.sh"
fi
if [ ! -f "$CORE_SCRIPT" ]; then
    echo "ERROR: start-zed-core.sh not found!"
    exit 1
fi

source "$CORE_SCRIPT"

# =========================================
# Run VS Code startup
# =========================================

start_vscode_helix
