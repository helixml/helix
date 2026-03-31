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
    local window_name
    window_name=$(echo "$title" | tr ' ' '-' | tr '[:upper:]' '[:lower:]')

    if ! tmux has-session -t "$tmux_session" 2>/dev/null; then
        # Create session with the command; drop to bash when done
        tmux new-session -d -s "$tmux_session" -n "$window_name" -x 80 -y 24 -c "$working_dir" \
            bash -c "$* ; exec bash -l"
        # Configure: hidden prefix, no status bar, mouse on
        tmux set-option -t "$tmux_session" prefix C-]
        tmux unbind-key -a 2>/dev/null || true
        tmux bind-key C-] send-prefix
        tmux set-option -t "$tmux_session" status off
        tmux set-option -t "$tmux_session" mouse on
        tmux set-option -t "$tmux_session" history-limit 10000
    else
        # Session exists — run command in a new window; drop to bash when done
        tmux new-window -t "$tmux_session" -n "$window_name" -c "$working_dir" \
            bash -c "$* ; exec bash -l"
    fi

    # In desktop mode, open ghostty attached to the specific window
    if [ "${HELIX_INTERFACE_MODE:-desktop}" = "desktop" ]; then
        # Select the window we just created, then attach
        tmux select-window -t "$tmux_session:$window_name" 2>/dev/null
        ghostty --gtk-single-instance=false --title="$title" --working-directory="$working_dir" \
            -e tmux attach-session -t "$tmux_session" &
    fi
    # In terminal mode: no ghostty. WebSocket PTY attaches to the tmux session.
    # Return the tmux server PID for health checks (always running).
    TERMINAL_PID=$(tmux display-message -t "$tmux_session" -p '#{pid}' 2>/dev/null || echo "$$")
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
