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
# Terminal launcher (Ubuntu uses ghostty)
# =========================================

launch_terminal() {
    local title="$1"
    local working_dir="$2"
    shift 2
    # Remaining args are the command
    # Ghostty options: --title, --working-directory, -e for command
    # CRITICAL: --gtk-single-instance=false prevents D-Bus activation which loses our -e args
    # MESA_GL_VERSION_OVERRIDE: Ghostty disables GLES (GDK_DISABLE=gles-api) and needs
    # OpenGL 3.3+ core. virgl under-reports as GL 2.1 but actually supports 4.5 features
    # (host Metal has full capability). Override lets Ghostty use hardware virgl instead of
    # falling back to llvmpipe software rendering.
    MESA_GL_VERSION_OVERRIDE=4.5 MESA_GLSL_VERSION_OVERRIDE=450 ghostty --gtk-single-instance=false --title="$title" --working-directory="$working_dir" -e "$@" &
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
