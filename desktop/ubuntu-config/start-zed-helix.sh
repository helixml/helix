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
    # Software rendering for virtio-gpu is handled by the ghostty wrapper (Dockerfile.ubuntu-helix).
    ghostty --gtk-single-instance=false --title="$title" --working-directory="$working_dir" -e "$@" &
}

# =========================================
# Initial window bounds — sidestep GNOME auto-maximize
# =========================================
#
# Upstream Zed commit a0d0195ca9 (PR #52940, 2026-04-07, merged into Helix via
# the 001864 Zed merge on 2026-04-24) bumped DEFAULT_WINDOW_SIZE from
# 1536x864 to 1536x1095. On a 1920x1080 virtual monitor, default_bounds()
# clips the new value to 1536x1080 — exactly the work-area height — which
# trips Mutter's auto-maximize=true and the user sees a fullscreen Zed.
#
# ZED_WINDOW_SIZE / ZED_WINDOW_POSITION (workspace.rs:171-183, :8011-8017)
# wrap the bounds as WindowBounds::Windowed and skip the auto-maximize path.
# Sized to 80% of the logical work area with a 10% margin on each side, so
# the proportions hold for 1920x1080, 4K, 5K and any HiDPI session.
#
# GDK_SCALE is exported by startup-app.sh when HELIX_ZOOM_LEVEL > 100;
# unset at 100% zoom (hence :-1). HELIX_SCALE_FACTOR itself isn't exported.
# At 1920x1080 / 100% the formula lands on 1536x864 — the exact size Zed
# defaulted to before a0d0195ca9.
ZED_SCALE=${GDK_SCALE:-1}
ZED_LOGICAL_W=$(( ${GAMESCOPE_WIDTH:-1920} / ZED_SCALE ))
ZED_LOGICAL_H=$(( ${GAMESCOPE_HEIGHT:-1080} / ZED_SCALE ))
ZED_W=$(( ZED_LOGICAL_W * 80 / 100 ))
ZED_H=$(( ZED_LOGICAL_H * 80 / 100 ))
ZED_X=$(( ZED_LOGICAL_W * 10 / 100 ))
ZED_Y=$(( ZED_LOGICAL_H * 10 / 100 ))
export ZED_WINDOW_SIZE="${ZED_WINDOW_SIZE:-${ZED_W},${ZED_H}}"
export ZED_WINDOW_POSITION="${ZED_WINDOW_POSITION:-${ZED_X},${ZED_Y}}"
echo "Zed initial window: size=${ZED_WINDOW_SIZE} position=${ZED_WINDOW_POSITION} (GAMESCOPE=${GAMESCOPE_WIDTH:-1920}x${GAMESCOPE_HEIGHT:-1080}, GDK_SCALE=${ZED_SCALE})"

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
