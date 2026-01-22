#!/bin/bash
#
# start-vscode-helix.sh - VS Code + Roo Code startup script for Ubuntu GNOME desktop
#
# This script sources start-zed-core.sh for common workspace setup logic,
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
# Source shared core for workspace setup
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

# =========================================
# VS Code-specific startup (replaces start_zed_helix)
# =========================================

start_vscode_helix() {
    echo "========================================="
    echo "Helix Agent Startup (${HELIX_DESKTOP_NAME}) - $(date)"
    echo "========================================="
    echo ""

    # Prevent duplicate instances (reuse pattern from core)
    VSCODE_LOCK_FILE="/tmp/helix-vscode-startup.lock"
    if [ -f "$VSCODE_LOCK_FILE" ]; then
        OLD_PID=$(cat "$VSCODE_LOCK_FILE" 2>/dev/null)
        if [ -n "$OLD_PID" ] && kill -0 "$OLD_PID" 2>/dev/null; then
            echo "VS Code startup already running (PID $OLD_PID) - exiting duplicate"
            exit 0
        fi
        echo "Stale lock file found, removing..."
        rm -f "$VSCODE_LOCK_FILE"
    fi
    echo $$ > "$VSCODE_LOCK_FILE"
    trap 'rm -f "$VSCODE_LOCK_FILE"' EXIT

    # Clean up old signal files
    rm -f "$COMPLETE_SIGNAL"

    # Find shared scripts (reuse from core)
    SHARED_SCRIPT_DIR="/usr/local/bin"
    if [ ! -f "$SHARED_SCRIPT_DIR/helix-workspace-setup.sh" ]; then
        SHARED_SCRIPT_DIR="/helix-dev/shared"
    fi
    if [ ! -f "$SHARED_SCRIPT_DIR/helix-workspace-setup.sh" ]; then
        echo "ERROR: helix-workspace-setup.sh not found!"
        exit 1
    fi

    echo "Using shared scripts from: $SHARED_SCRIPT_DIR"
    echo ""

    # Ensure work directory exists
    mkdir -p "$WORK_DIR"

    # =========================================
    # Step 1: Run workspace setup in terminal (BLOCKS until complete)
    # =========================================
    echo "Launching setup terminal..."
    launch_terminal "Helix Setup" "$WORK_DIR" bash "$SHARED_SCRIPT_DIR/helix-workspace-setup.sh"
    echo "Setup terminal launched"

    # Use shared wait function from core
    wait_for_setup_complete

    # =========================================
    # Step 2: Read folders to open (same as Zed)
    # =========================================
    # The workspace setup writes folders to ~/.helix-zed-folders
    # We read them and pass to VS Code (multi-root workspace support)
    VSCODE_FOLDERS=()
    if [ -f "$FOLDERS_FILE" ]; then
        while IFS= read -r folder; do
            if [ -n "$folder" ] && [ -d "$folder" ]; then
                VSCODE_FOLDERS+=("$folder")
            fi
        done < "$FOLDERS_FILE"
    fi

    # If no folders, fall back to work directory
    if [ ${#VSCODE_FOLDERS[@]} -eq 0 ]; then
        echo "No folders found in $FOLDERS_FILE, falling back to $WORK_DIR"
        VSCODE_FOLDERS=("$WORK_DIR")
    else
        echo "Opening VS Code with ${#VSCODE_FOLDERS[@]} folder(s):"
        for folder in "${VSCODE_FOLDERS[@]}"; do
            echo "  - $(basename "$folder")"
        done
    fi

    # =========================================
    # Step 3: Configure Roo Code extension
    # =========================================
    # ROO_CODE_API_URL tells the extension where to connect
    # The RooCodeBridge in desktop-bridge serves the config on port 9879
    export ROO_CODE_API_URL="http://localhost:9879"
    echo "ROO_CODE_API_URL=$ROO_CODE_API_URL"

    # =========================================
    # Step 4: Launch VS Code with auto-restart
    # =========================================
    echo "Starting VS Code with auto-restart loop..."

    # Trap signals to prevent script exit when VS Code is closed
    trap 'echo "Caught signal, continuing restart loop..."' 15 2 1

    while true; do
        echo "Launching VS Code..."
        # --disable-workspace-trust: Skip the trust dialog for mounted workspaces
        # Pass all folders - VS Code opens them as a multi-root workspace
        code --disable-workspace-trust "${VSCODE_FOLDERS[@]}" || true
        echo "VS Code exited, restarting in 2 seconds..."
        sleep 2
    done
}

# Run the VS Code startup
start_vscode_helix
