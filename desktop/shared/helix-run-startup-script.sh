#!/bin/bash
#
# helix-run-startup-script.sh - Run user's project startup script (MANUAL USE ONLY)
#
# NOTE: As of 2026-01, this script is no longer launched automatically.
# The startup script is now run by helix-workspace-setup.sh in the same terminal.
# This file is kept for manual re-runs if needed.
#
# This script runs the project's .helix/startup.sh from the trusted helix-specs
# commit (HELIX_TRUSTED_STARTUP_SHA; see helix-trusted-startup.sh), never from
# the live helix-specs worktree.
#
# The startup script is typically used for:
# - Installing dependencies (npm install, pip install, etc.)
# - Starting development servers
# - Running builds
#
# Usage: Run manually in a terminal if you need to re-run the startup script

echo "========================================="
echo "Project Startup Script - $(date)"
echo "========================================="
echo ""

WORK_DIR="$HOME/work"
cd "$WORK_DIR"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [ -f "$SCRIPT_DIR/helix-trusted-startup.sh" ]; then
    source "$SCRIPT_DIR/helix-trusted-startup.sh"
else
    source /usr/local/bin/helix-trusted-startup.sh
fi

HELIX_SPECS_DIR="$WORK_DIR/helix-specs"
STARTUP_SCRIPT_PATH=""
STARTUP_STATUS=1
if [ -n "$HELIX_PRIMARY_REPO_NAME" ]; then
    STARTUP_SCRIPT_PATH=$(helix_trusted_startup_script "$WORK_DIR/$HELIX_PRIMARY_REPO_NAME" "$HELIX_SPECS_DIR")
    STARTUP_STATUS=$?
fi

if [ "$STARTUP_STATUS" -gt 1 ]; then
    echo ""
elif [ "$STARTUP_STATUS" -eq 0 ]; then
    echo "Script: $STARTUP_SCRIPT_PATH"
    echo ""

    # Change to primary repository for running commands
    if [ -n "$HELIX_PRIMARY_REPO_NAME" ]; then
        PRIMARY_REPO_PATH="$WORK_DIR/$HELIX_PRIMARY_REPO_NAME"
        if [ -d "$PRIMARY_REPO_PATH" ]; then
            cd "$PRIMARY_REPO_PATH"
            echo "Working in: $HELIX_PRIMARY_REPO_NAME"
        fi
    fi
    echo ""

    # Run the startup script
    if bash -i "$STARTUP_SCRIPT_PATH"; then
        echo ""
        echo "========================================="
        echo "Startup script completed successfully"
        echo "========================================="
    else
        EXIT_CODE=$?
        echo ""
        echo "========================================="
        echo "Startup script failed with exit code $EXIT_CODE"
        echo "========================================="
        echo ""
        echo "To fix this:"
        echo "  1. Edit the startup script in Project Settings"
        echo "  2. Click 'Test Startup Script' to test your changes"
    fi
    echo ""
else
    if [ -d "$HELIX_SPECS_DIR" ]; then
        echo "No startup script in the trusted helix-specs commit"
        echo "Add a startup script in Project Settings to run setup commands"
    elif [ -n "$HELIX_PRIMARY_REPO_NAME" ]; then
        echo "No helix-specs worktree found"
        echo "Startup script should be in helix-specs/.helix/startup.sh"
    else
        echo "No startup script - no primary repository configured"
    fi
    echo ""
fi

echo "========================================="
echo "What would you like to do?"
echo "========================================="
echo "  1) Close this window"
echo "  2) Start an interactive shell"
echo ""
read -p "Enter choice [1-2]: " choice

case "$choice" in
    1)
        echo "Closing..."
        exit 0
        ;;
    2|*)
        echo ""
        echo "Starting interactive shell..."
        echo "Type 'exit' to close this window."
        echo ""
        # Go to primary repo if it exists
        if [ -n "$HELIX_PRIMARY_REPO_NAME" ] && [ -d "$WORK_DIR/$HELIX_PRIMARY_REPO_NAME" ]; then
            cd "$WORK_DIR/$HELIX_PRIMARY_REPO_NAME"
        else
            cd "$WORK_DIR"
        fi
        exec bash
        ;;
esac
