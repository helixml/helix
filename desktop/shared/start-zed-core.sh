#!/bin/bash
#
# start-zed-core.sh - Core Zed startup logic (shared between desktops)
#
# This script contains the common startup logic for all desktops.
# It should be sourced by desktop-specific start-zed-helix.sh scripts.
#
# Desktop-specific scripts must:
# 1. Set HELIX_DESKTOP_NAME (for logging, e.g., "Sway" or "Ubuntu GNOME")
# 2. Define launch_terminal() function
# 3. Optionally set ZED_EXTRA_FILES array (e.g., user guide)
# 4. Optionally define pre_zed_launch() hook
# 5. Source this script
# 6. Call start_zed_helix
#
# Required function to be defined by caller:
#   launch_terminal <title> <working_dir> <command...>
#       Launch a terminal window in background with given title, working dir, and command

# Define paths
WORK_DIR="$HOME/work"
COMPLETE_SIGNAL="$HOME/.helix-setup-complete"
FOLDERS_FILE="$HOME/.helix-zed-folders"

# Will be populated by read_zed_folders
ZED_FOLDERS=()

# =========================================
# Helper functions
# =========================================

wait_for_setup_complete() {
    echo "Waiting for workspace setup to complete..."
    local WAIT_COUNT=0
    local MAX_WAIT=300  # 5 minutes max wait

    while [ ! -f "$COMPLETE_SIGNAL" ]; do
        sleep 1
        WAIT_COUNT=$((WAIT_COUNT + 1))
        if [ $((WAIT_COUNT % 30)) -eq 0 ]; then
            echo "Still waiting for setup... ($WAIT_COUNT seconds)"
        fi
        if [ $WAIT_COUNT -ge $MAX_WAIT ]; then
            echo "Warning: Setup timeout after ${MAX_WAIT}s, proceeding anyway..."
            return 1
        fi
    done

    echo "Setup complete"
    return 0
}

wait_for_zed_config() {
    echo "Waiting for Zed configuration..."
    local WAIT_COUNT=0
    local MAX_WAIT=30

    while [ $WAIT_COUNT -lt $MAX_WAIT ]; do
        if [ -f "$HOME/.config/zed/settings.json" ]; then
            if grep -q '"default_model"' "$HOME/.config/zed/settings.json" 2>/dev/null; then
                echo "Zed configuration ready"
                return 0
            fi
        fi
        sleep 1
        WAIT_COUNT=$((WAIT_COUNT + 1))
        if [ $((WAIT_COUNT % 10)) -eq 0 ]; then
            echo "Still waiting for settings.json... ($WAIT_COUNT seconds)"
        fi
    done

    echo "Warning: Settings not ready after ${MAX_WAIT}s, proceeding anyway..."
    return 1
}

read_zed_folders() {
    ZED_FOLDERS=()
    if [ -f "$FOLDERS_FILE" ]; then
        while IFS= read -r folder; do
            if [ -n "$folder" ] && [ -d "$folder" ]; then
                ZED_FOLDERS+=("$folder")
            fi
        done < "$FOLDERS_FILE"
    fi

    # If no folders, setup failed - don't start Zed
    if [ ${#ZED_FOLDERS[@]} -eq 0 ]; then
        echo "ERROR: No folders to open - setup may have failed"
        echo "Check the setup terminal for errors"
        exit 1
    else
        echo "Opening Zed with ${#ZED_FOLDERS[@]} folder(s):"
        for folder in "${ZED_FOLDERS[@]}"; do
            echo "  - $(basename "$folder")"
        done
    fi
}

launch_acp_log_viewer() {
    if [ "$SHOW_ACP_DEBUG_LOGS" = "true" ] || [ -n "$HELIX_DEBUG" ]; then
        echo "Starting ACP log viewer..."
        launch_terminal "ACP Agent Logs" "$WORK_DIR" bash -c '
            echo "ACP Agent Log Viewer - Tailing Zed logs"
            echo ""
            while [ ! -d ~/.local/share/zed/logs ]; do sleep 1; done
            tail -F ~/.local/share/zed/logs/*.log 2>/dev/null
        '
    fi
}

run_zed_restart_loop() {
    echo "Starting Zed with auto-restart loop..."

    # Trap signals to prevent script exit when Zed is closed
    trap 'echo "Caught signal, continuing restart loop..."' 15 2 1

    while true; do
        echo "Launching Zed..."
        # ZED_EXTRA_FILES can be set by desktop-specific script (e.g., user guide)
        /zed-build/zed "${ZED_FOLDERS[@]}" "${ZED_EXTRA_FILES[@]}" || true
        echo "Zed exited, restarting in 2 seconds..."
        sleep 2
    done
}

# =========================================
# Main startup sequence
# =========================================

start_zed_helix() {
    echo "========================================="
    echo "Helix Agent Startup (${HELIX_DESKTOP_NAME:-Unknown}) - $(date)"
    echo "========================================="
    echo ""

    # Prevent duplicate Zed instances after compositor crash/restart
    # The compositor's exec command runs again on restart, but we're already running
    ZED_LOCK_FILE="/tmp/helix-zed-startup.lock"
    if [ -f "$ZED_LOCK_FILE" ]; then
        OLD_PID=$(cat "$ZED_LOCK_FILE" 2>/dev/null)
        if [ -n "$OLD_PID" ] && kill -0 "$OLD_PID" 2>/dev/null; then
            echo "Zed startup already running (PID $OLD_PID) - exiting duplicate"
            exit 0
        fi
        echo "Stale lock file found, removing..."
        rm -f "$ZED_LOCK_FILE"
    fi
    echo $$ > "$ZED_LOCK_FILE"
    trap 'rm -f "$ZED_LOCK_FILE"' EXIT

    # Clean up old signal files
    rm -f "$COMPLETE_SIGNAL" "$FOLDERS_FILE"

    # Check if Zed binary exists
    if [ ! -f "/zed-build/zed" ]; then
        echo "Zed binary not found at /zed-build/zed - cannot start Zed agent"
        exit 1
    fi

    # Find shared scripts (check /usr/local/bin first, then dev path)
    SHARED_SCRIPT_DIR="/usr/local/bin"
    if [ ! -f "$SHARED_SCRIPT_DIR/helix-workspace-setup.sh" ]; then
        SHARED_SCRIPT_DIR="/helix-dev/shared"
    fi
    if [ ! -f "$SHARED_SCRIPT_DIR/helix-workspace-setup.sh" ]; then
        echo "ERROR: helix-workspace-setup.sh not found!"
        echo "Checked: /usr/local/bin and /helix-dev/shared"
        exit 1
    fi

    echo "Using shared scripts from: $SHARED_SCRIPT_DIR"
    echo ""

    # Ensure work directory exists
    mkdir -p "$WORK_DIR"

    # =========================================
    # Step 1: Run workspace setup in terminal (BLOCKS until complete)
    # =========================================
    # The setup terminal runs:
    # - Git configuration
    # - Repository cloning (shows errors if clone fails)
    # - Branch checkout
    # - Startup script (if exists)
    # - Stays open as bash shell for debugging
    echo "Launching setup terminal..."
    launch_terminal "Helix Setup" "$WORK_DIR" bash "$SHARED_SCRIPT_DIR/helix-workspace-setup.sh"
    echo "Setup terminal launched"

    wait_for_setup_complete

    # =========================================
    # Step 2: Wait for Zed configuration
    # =========================================
    wait_for_zed_config

    # =========================================
    # Step 3: Launch Zed
    # =========================================
    read_zed_folders

    # Call optional pre-launch hook (desktop-specific setup like env vars, extra files)
    if type pre_zed_launch &>/dev/null; then
        pre_zed_launch
    fi

    launch_acp_log_viewer
    run_zed_restart_loop
}
