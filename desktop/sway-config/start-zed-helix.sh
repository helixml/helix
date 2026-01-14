#!/bin/bash
#
# start-zed-helix.sh - Zed startup script for Sway desktop
#
# This script orchestrates the Helix workspace startup:
# 1. Launch terminal showing workspace setup (cloning, git config) - BLOCKS until done
# 2. Launch terminal running user's startup script - BACKGROUND (parallel with Zed)
# 3. Launch Zed with the workspace folders
#
# The heavy lifting is done by shared scripts in /usr/local/bin:
# - helix-workspace-setup.sh: Git config, cloning, worktree setup
# - helix-run-startup-script.sh: User's project startup.sh

echo "========================================="
echo "Helix Agent Startup (Sway) - $(date)"
echo "========================================="
echo ""

# Define paths
WORK_DIR="$HOME/work"
COMPLETE_SIGNAL="$HOME/.helix-setup-complete"
FOLDERS_FILE="$HOME/.helix-zed-folders"

# Clean up old signal files
rm -f "$COMPLETE_SIGNAL" "$FOLDERS_FILE"

# Check if Zed binary exists
if [ ! -f "/zed-build/zed" ]; then
    echo "Zed binary not found at /zed-build/zed - cannot start Zed agent"
    exit 1
fi

# Verify WAYLAND_DISPLAY is set by Sway
if [ -z "$WAYLAND_DISPLAY" ]; then
    echo "ERROR: WAYLAND_DISPLAY not set! Sway should set this automatically."
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
echo "Launching workspace setup terminal..."

kitty --title="Helix Workspace Setup" \
    --directory="$WORK_DIR" \
    bash "$SHARED_SCRIPT_DIR/helix-workspace-setup.sh" &

SETUP_TERMINAL_PID=$!
echo "Setup terminal launched (PID: $SETUP_TERMINAL_PID)"

# Wait for setup to complete (signaled by COMPLETE_SIGNAL file)
echo "Waiting for workspace setup to complete..."
WAIT_COUNT=0
MAX_WAIT=300  # 5 minutes max wait

while [ ! -f "$COMPLETE_SIGNAL" ]; do
    sleep 1
    WAIT_COUNT=$((WAIT_COUNT + 1))
    if [ $((WAIT_COUNT % 30)) -eq 0 ]; then
        echo "Still waiting for setup... ($WAIT_COUNT seconds)"
    fi
    if [ $WAIT_COUNT -ge $MAX_WAIT ]; then
        echo "Warning: Setup timeout after ${MAX_WAIT}s, proceeding anyway..."
        break
    fi
done

if [ -f "$COMPLETE_SIGNAL" ]; then
    echo "Setup complete"
fi

# =========================================
# Step 2: Run user's startup script in background terminal
# =========================================
# Check if startup script exists before launching terminal
STARTUP_SCRIPT="$WORK_DIR/helix-specs/.helix/startup.sh"
if [ -f "$STARTUP_SCRIPT" ]; then
    echo "Launching startup script terminal (background)..."

    kitty --title="Project Startup Script" \
        --directory="$WORK_DIR" \
        bash "$SHARED_SCRIPT_DIR/helix-run-startup-script.sh" &

    echo "Startup script terminal launched"
else
    echo "No startup script found - skipping"
fi

# =========================================
# Step 3: Wait for Zed configuration
# =========================================
echo "Waiting for Zed configuration..."
WAIT_COUNT=0
MAX_WAIT=30

while [ $WAIT_COUNT -lt $MAX_WAIT ]; do
    if [ -f "$HOME/.config/zed/settings.json" ]; then
        if grep -q '"default_model"' "$HOME/.config/zed/settings.json" 2>/dev/null; then
            echo "Zed configuration ready"
            break
        fi
    fi
    sleep 1
    WAIT_COUNT=$((WAIT_COUNT + 1))
    if [ $((WAIT_COUNT % 10)) -eq 0 ]; then
        echo "Still waiting for settings.json... ($WAIT_COUNT seconds)"
    fi
done

if [ $WAIT_COUNT -ge $MAX_WAIT ]; then
    echo "Warning: Settings not ready after ${MAX_WAIT}s, proceeding anyway..."
fi

# =========================================
# Step 4: Launch Zed
# =========================================
# Trap signals to prevent script exit when Zed is closed
trap 'echo "Caught signal, continuing restart loop..."' 15 2 1

# Read folders from file written by setup script
ZED_FOLDERS=()
if [ -f "$FOLDERS_FILE" ]; then
    while IFS= read -r folder; do
        if [ -n "$folder" ] && [ -d "$folder" ]; then
            ZED_FOLDERS+=("$folder")
        fi
    done < "$FOLDERS_FILE"
fi

# Fallback to work directory if no folders
if [ ${#ZED_FOLDERS[@]} -eq 0 ]; then
    ZED_FOLDERS=("$WORK_DIR")
    echo "Opening Zed in work directory (no folders from setup)"
else
    echo "Opening Zed with ${#ZED_FOLDERS[@]} folder(s):"
    for folder in "${ZED_FOLDERS[@]}"; do
        echo "  - $(basename "$folder")"
    done
fi

# Add Sway user guide if it exists
USER_GUIDE_PATH="$WORK_DIR/SWAY-USER-GUIDE.md"
if [ -f "$USER_GUIDE_PATH" ]; then
    echo "  + SWAY-USER-GUIDE.md"
fi

# Launch ACP log viewer if debug mode enabled
if [ "$SHOW_ACP_DEBUG_LOGS" = "true" ] || [ -n "$HELIX_DEBUG" ]; then
    echo "Starting ACP log viewer..."
    kitty --class acp-log-viewer \
        --title "ACP Agent Logs" \
        -e bash -c '
            echo "ACP Agent Log Viewer - Tailing Zed logs"
            echo ""
            while [ ! -d ~/.local/share/zed/logs ]; do sleep 1; done
            tail -F ~/.local/share/zed/logs/*.log 2>/dev/null
        ' &
fi

echo "Starting Zed with auto-restart loop..."
echo "Using Wayland backend (WAYLAND_DISPLAY=$WAYLAND_DISPLAY)"

while true; do
    echo "Launching Zed..."
    if [ -f "$USER_GUIDE_PATH" ]; then
        /zed-build/zed "${ZED_FOLDERS[@]}" "$USER_GUIDE_PATH" || true
    else
        /zed-build/zed "${ZED_FOLDERS[@]}" || true
    fi
    echo "Zed exited, restarting in 2 seconds..."
    sleep 2
done
