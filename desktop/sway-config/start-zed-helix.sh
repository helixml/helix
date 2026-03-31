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

    # Always run commands in a shared tmux session (helix-shell).
    local tmux_session="helix-shell"
    local window_name
    window_name=$(echo "$title" | tr ' ' '-' | tr '[:upper:]' '[:lower:]')

    if ! tmux has-session -t "$tmux_session" 2>/dev/null; then
        tmux new-session -d -s "$tmux_session" -n "$window_name" -x 80 -y 24 -c "$working_dir" \
            bash -c "$* ; exec bash -l"
        tmux set-option -t "$tmux_session" prefix C-]
        tmux unbind-key -a 2>/dev/null || true
        tmux bind-key C-] send-prefix
        tmux set-option -t "$tmux_session" status off
        tmux set-option -t "$tmux_session" mouse on
        tmux set-option -t "$tmux_session" history-limit 10000
    else
        tmux new-window -t "$tmux_session" -n "$window_name" -c "$working_dir" \
            bash -c "$* ; exec bash -l"
    fi

    if [ "${HELIX_INTERFACE_MODE:-desktop}" = "desktop" ]; then
        tmux select-window -t "$tmux_session:$window_name" 2>/dev/null
        kitty --title="$title" --directory="$working_dir" tmux attach-session -t "$tmux_session" &
    fi
    TERMINAL_PID=$(tmux display-message -t "$tmux_session" -p '#{pid}' 2>/dev/null || echo "$$")
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
