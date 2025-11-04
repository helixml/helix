#!/bin/bash
# Startup script for Zed editor connected to Helix controlplane (Sway version)
set -e

# Check if Zed binary exists (directory mounted to survive inode changes on rebuild)
if [ ! -f "/zed-build/zed" ]; then
    echo "Zed binary not found at /zed-build/zed - cannot start Zed agent"
    exit 1
fi

# Environment variables are passed from Wolf executor via container env
# HELIX_API_URL, HELIX_API_TOKEN, ANTHROPIC_API_KEY should be available

# NOTE: Zed state symlinks are already created by startup-app.sh BEFORE Sway starts
# This ensures settings-sync-daemon can write config.json immediately on startup

# Set workspace to mounted work directory
WORK_DIR=/home/retro/work
cd $WORK_DIR

# Create Claude Code state symlink if needed
CLAUDE_STATE_DIR=$WORK_DIR/.claude-state
if command -v claude &> /dev/null; then
    mkdir -p $CLAUDE_STATE_DIR
    rm -rf ~/.claude
    ln -sf $CLAUDE_STATE_DIR ~/.claude
    echo "✅ Claude: ~/.claude → $CLAUDE_STATE_DIR"
fi

# Initialize workspace with README if empty
# This ensures Zed creates a workspace and triggers WebSocket connection
if [ ! -f "README.md" ] && [ -z "$(ls -A)" ]; then
    cat > README.md << 'HEREDOC'
# Welcome to Your Helix External Agent

This is your autonomous development workspace. The AI agent running in this environment
can read and write files, run commands, and collaborate with you through the Helix interface.

## Getting Started

- This workspace is persistent across sessions
- Files you create here are saved automatically
- The AI agent has full access to this directory
- Use the Helix chat interface to direct the agent

## Directories

Create your project structure here. For example:
```
mkdir src
mkdir tests
```

Start coding and the agent will assist you!
HEREDOC
    echo "Created README.md to initialize workspace"
fi

# Configure SSH agent and load keys for git access
if [ -d "/home/retro/.ssh" ] && [ "$(ls -A /home/retro/.ssh/*.key 2>/dev/null)" ]; then
    echo "Setting up SSH agent for git access..."
    eval "$(ssh-agent -s)"
    for key in /home/retro/.ssh/*.key; do
        ssh-add "$key" 2>/dev/null && echo "Loaded SSH key: $(basename $key)"
    done
fi

# Configure git from environment variables if provided
if [ -n "$GIT_USER_NAME" ]; then
    git config --global user.name "$GIT_USER_NAME"
    echo "Configured git user.name: $GIT_USER_NAME"
fi

if [ -n "$GIT_USER_EMAIL" ]; then
    git config --global user.email "$GIT_USER_EMAIL"
    echo "Configured git user.email: $GIT_USER_EMAIL"
fi

# Execute project startup script if provided - run in terminal window
if [ -n "$HELIX_PROJECT_STARTUP_SCRIPT" ]; then
    echo "========================================="
    echo "Running project startup script in terminal..."
    echo "========================================="

    # Write startup script to temp file
    STARTUP_SCRIPT_FILE="$WORK_DIR/.helix-startup.sh"
    echo "$HELIX_PROJECT_STARTUP_SCRIPT" > "$STARTUP_SCRIPT_FILE"
    chmod +x "$STARTUP_SCRIPT_FILE"

    # Create wrapper script that runs the startup script and handles errors
    WRAPPER_SCRIPT="$WORK_DIR/.helix-startup-wrapper.sh"
    cat > "$WRAPPER_SCRIPT" << 'WRAPPER_EOF'
#!/bin/bash
echo "========================================="
echo "Running Project Startup Script..."
echo "========================================="
echo ""

# Run the startup script with timeout (5 minutes max)
if timeout 300 bash "$WORK_DIR/.helix-startup.sh"; then
    echo ""
    echo "========================================="
    echo "✅ Startup script completed successfully"
    echo "========================================="
    echo ""
    echo "Press Enter to close this window..."
    read
else
    EXIT_CODE=$?
    echo ""
    echo "========================================="
    if [ $EXIT_CODE -eq 124 ]; then
        echo "⚠️ Startup script timed out after 5 minutes"
    else
        echo "❌ Startup script failed with exit code $EXIT_CODE"
    fi
    echo "========================================="
    echo ""
    echo "Waiting 60 seconds so you can see the error..."
    for i in $(seq 60 -1 1); do
        echo -ne "Closing in $i seconds... \r"
        sleep 1
    done
    echo ""
fi
WRAPPER_EOF
    chmod +x "$WRAPPER_SCRIPT"

    # Launch terminal in background to run the wrapper script
    # Use ghostty terminal emulator
    ghostty --title="Project Startup Script" \
            --working-directory="$WORK_DIR" \
            bash "$WRAPPER_SCRIPT" &

    echo "Startup script terminal launched (check right side of screen)"
fi

# Wait for settings-sync-daemon to create configuration
# Check for agent.default_model which is critical for Zed to work
echo "Waiting for Zed configuration to be initialized..."
WAIT_COUNT=0
MAX_WAIT=30  # Reduced to 30 seconds since daemon usually syncs quickly

while [ $WAIT_COUNT -lt $MAX_WAIT ]; do
    if [ -f "$HOME/.config/zed/settings.json" ]; then
        # Check if settings.json has agent.default_model configured
        if grep -q '"default_model"' "$HOME/.config/zed/settings.json" 2>/dev/null; then
            echo "✅ Zed configuration ready with default_model"
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
    echo "⚠️  Warning: Settings not ready after ${MAX_WAIT}s, proceeding anyway..."
fi

# Trap signals to prevent script exit when Zed is closed
# Using signal numbers for compatibility: 15=TERM, 2=INT, 1=HUP
trap 'echo "Caught signal, continuing restart loop..."' 15 2 1

# Verify WAYLAND_DISPLAY is set by Sway (Zed needs this for native Wayland backend)
# Zed checks WAYLAND_DISPLAY - if empty, it falls back to Xwayland (which causes input issues with NVIDIA)
# Reference: https://github.com/zed-industries/zed/blob/main/docs/src/linux.md
if [ -z "$WAYLAND_DISPLAY" ]; then
    echo "ERROR: WAYLAND_DISPLAY not set! Sway should set this automatically."
    echo "Cannot start Zed without Wayland - would fall back to broken Xwayland."
    exit 1
fi

# Launch Zed in a restart loop for development
# When you close Zed (click X), it auto-restarts with the latest binary
# Perfect for testing rebuilds without recreating the entire container
echo "Starting Zed with auto-restart loop (close window to reload updated binary)"
echo "Using Wayland backend (WAYLAND_DISPLAY=$WAYLAND_DISPLAY)"
while true; do
    echo "Launching Zed..."
    /zed-build/zed . || true
    echo "Zed exited, restarting in 2 seconds..."
    sleep 2
done
