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

    # Always run commands in a shared tmux session (helix-shell).
    # This allows both desktop mode (ghostty attaches) and terminal mode
    # (WebSocket PTY attaches) to share the same persistent session.
    local tmux_session="helix-shell"

    if ! tmux has-session -t "$tmux_session" 2>/dev/null; then
        # Create session with the command
        tmux new-session -d -s "$tmux_session" -x 80 -y 24 -c "$working_dir" "$@"
        # Configure: hidden prefix, no status bar, mouse on
        tmux set-option -t "$tmux_session" -g prefix C-]
        tmux unbind -t "$tmux_session" C-b 2>/dev/null || true
        tmux set-option -t "$tmux_session" -g status off
        tmux set-option -t "$tmux_session" -g mouse on
        tmux set-option -t "$tmux_session" -g history-limit 10000
    else
        # Session exists — run command in a new window
        tmux new-window -t "$tmux_session" -n "$title" -c "$working_dir" "$@"
    fi

    # In desktop mode, open ghostty attached to the tmux session
    if [ "${HELIX_INTERFACE_MODE:-desktop}" = "desktop" ]; then
        ghostty --gtk-single-instance=false --title="$title" --working-directory="$working_dir" \
            -e tmux attach-session -t "$tmux_session" &
    fi
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
