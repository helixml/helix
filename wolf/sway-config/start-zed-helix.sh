#!/bin/bash
# Startup script for Zed editor connected to Helix controlplane (Sway version)
#
# This script launches a kitty terminal for ALL setup work so the user can see
# what's happening. The setup runs inside kitty, then signals completion so
# Zed can launch with the correct folders.

# Don't use set -e here - we need to handle failures gracefully
# set -e

# Redirect all output to log file AND stdout (using tee)
STARTUP_LOG="$HOME/.helix-startup.log"
exec > >(tee "$STARTUP_LOG") 2>&1

echo "========================================="
echo "Helix Agent Startup v4 - $(date)"
echo "========================================="
echo ""

# Define paths for inter-process communication
WORK_DIR="$HOME/work"
COMPLETE_SIGNAL="$HOME/.helix-startup-complete"
FOLDERS_FILE="$HOME/.helix-zed-folders"
SETUP_SCRIPT="$WORK_DIR/.helix-setup.sh"

# Clean up old signal files from previous runs
rm -f "$COMPLETE_SIGNAL" "$FOLDERS_FILE"

# Check if Zed binary exists (directory mounted to survive inode changes on rebuild)
if [ ! -f "/zed-build/zed" ]; then
    echo "Zed binary not found at /zed-build/zed - cannot start Zed agent"
    exit 1
fi

# Verify WAYLAND_DISPLAY is set by Sway (Zed needs this for native Wayland backend)
if [ -z "$WAYLAND_DISPLAY" ]; then
    echo "ERROR: WAYLAND_DISPLAY not set! Sway should set this automatically."
    echo "Cannot start Zed without Wayland - would fall back to broken Xwayland."
    exit 1
fi

# Ensure work directory exists
mkdir -p "$WORK_DIR"

# Get the directory where this script lives (for sourcing helper scripts)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Create the setup script that runs inside kitty
# This script does ALL the work so the user can see progress and errors
cat > "$SETUP_SCRIPT" <<'SETUP_SCRIPT_EOF'
#!/bin/bash
# Helix Workspace Setup Script - runs inside kitty terminal
# All output is visible to the user

echo "========================================="
echo "Helix Workspace Setup - $(date)"
echo "========================================="
echo ""

# Source the git helper functions (provides get_default_branch, create_helix_specs_branch)
# This handles Azure DevOps repos that don't set refs/remotes/origin/HEAD
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [ -f "$SCRIPT_DIR/helix-specs-create.sh" ]; then
    source "$SCRIPT_DIR/helix-specs-create.sh"
elif [ -f "/usr/local/bin/helix-specs-create.sh" ]; then
    source "/usr/local/bin/helix-specs-create.sh"
else
    echo "⚠️  helix-specs-create.sh not found - using fallback branch detection"
    # Fallback implementation if helper not found
    get_default_branch() {
        local REPO_PATH="$1"
        local BRANCH=$(git -C "$REPO_PATH" symbolic-ref refs/remotes/origin/HEAD 2>/dev/null | sed 's@^refs/remotes/origin/@@')
        if [ -z "$BRANCH" ]; then
            if git -C "$REPO_PATH" show-ref --verify refs/remotes/origin/main >/dev/null 2>&1; then
                BRANCH="main"
            elif git -C "$REPO_PATH" show-ref --verify refs/remotes/origin/master >/dev/null 2>&1; then
                BRANCH="master"
            else
                BRANCH="main"
            fi
        fi
        echo "$BRANCH"
    }
fi

# Debug: Show key environment variables (sanitized)
if [ -n "$USER_API_TOKEN" ]; then
    echo "✅ USER_API_TOKEN: ${USER_API_TOKEN:0:8}..."
else
    echo "❌ USER_API_TOKEN: not set"
fi

if [ -n "$HELIX_REPOSITORIES" ]; then
    echo "✅ HELIX_REPOSITORIES: $HELIX_REPOSITORIES"
else
    echo "❌ HELIX_REPOSITORIES: not set"
fi

if [ -n "$HELIX_PRIMARY_REPO_NAME" ]; then
    echo "✅ HELIX_PRIMARY_REPO_NAME: $HELIX_PRIMARY_REPO_NAME"
else
    echo "❌ HELIX_PRIMARY_REPO_NAME: not set"
fi
echo ""

# Set workspace directory
WORK_DIR="$HOME/work"
cd "$WORK_DIR"

# Signal files for communication with main script
COMPLETE_SIGNAL="$HOME/.helix-startup-complete"
FOLDERS_FILE="$HOME/.helix-zed-folders"

# Track which folders should be opened in Zed
declare -a ZED_FOLDERS

# =========================================
# Git Configuration
# =========================================
echo "========================================="
echo "Configuring Git..."
echo "========================================="

# Configure git user identity (required for commits)
if [ -n "$GIT_USER_NAME" ]; then
    git config --global user.name "$GIT_USER_NAME"
    echo "✅ Git user.name: $GIT_USER_NAME"
else
    git config --global user.name "Helix Agent"
    echo "⚠️ Git user.name: Helix Agent (default - should have been set by executor)"
fi

# CRITICAL: Enterprise ADO deployments reject commits from non-corporate email addresses
# The wolf_executor MUST always set GIT_USER_EMAIL - missing is a bug
if [ -n "$GIT_USER_EMAIL" ]; then
    git config --global user.email "$GIT_USER_EMAIL"
    echo "✅ Git user.email: $GIT_USER_EMAIL"
else
    echo "❌ FATAL: GIT_USER_EMAIL not set"
    echo "   Enterprise ADO deployments reject commits from non-corporate email addresses"
    echo "   This is a bug in wolf_executor - it should always pass GIT_USER_EMAIL"
    exit 1
fi

# Configure git to use merge commits (not rebase) for concurrent agent work
git config --global pull.rebase false
echo "✅ Git pull strategy: merge (for concurrent agent compatibility)"

# Configure git credentials for HTTP operations (MUST happen before cloning!)
if [ -n "$USER_API_TOKEN" ] && [ -n "$HELIX_API_BASE_URL" ]; then
    git config --global credential.helper 'store --file ~/.git-credentials'
    GIT_API_HOST=$(echo "$HELIX_API_BASE_URL" | sed 's|^https\?://||')
    GIT_API_PROTOCOL=$(echo "$HELIX_API_BASE_URL" | grep -o '^https\?' || echo "http")
    echo "${GIT_API_PROTOCOL}://api:${USER_API_TOKEN}@${GIT_API_HOST}" > ~/.git-credentials
    chmod 600 ~/.git-credentials
    echo "✅ Git credentials configured for $GIT_API_HOST"
else
    echo "⚠️  USER_API_TOKEN or HELIX_API_BASE_URL not set - git operations may fail"
fi
echo ""

# =========================================
# Clone Repositories
# =========================================
if [ -n "$HELIX_REPOSITORIES" ] && [ -n "$USER_API_TOKEN" ]; then
    echo "========================================="
    echo "Cloning project repositories..."
    echo "========================================="

    IFS=',' read -ra REPOS <<< "$HELIX_REPOSITORIES"
    for REPO_SPEC in "${REPOS[@]}"; do
        # Parse "id:name:type" format
        IFS=':' read -r REPO_ID REPO_NAME REPO_TYPE <<< "$REPO_SPEC"

        # Skip internal repos - they're deprecated
        if [ "$REPO_TYPE" = "internal" ]; then
            echo "📦 Skipping internal repo: $REPO_NAME (deprecated)"
            continue
        fi

        echo "📦 Repository: $REPO_NAME (type: $REPO_TYPE)"
        CLONE_DIR="$WORK_DIR/$REPO_NAME"

        # If already cloned, just skip (startup script is in helix-specs worktree)
        if [ -d "$CLONE_DIR/.git" ]; then
            echo "  ✅ Already cloned at $CLONE_DIR"
            continue
        fi

        # Clone repository using HTTP with credentials
        GIT_API_HOST=$(echo "$HELIX_API_BASE_URL" | sed 's|^https\?://||')
        GIT_API_PROTOCOL=$(echo "$HELIX_API_BASE_URL" | grep -o '^https\?' || echo "http")
        echo "  📥 Cloning from ${GIT_API_PROTOCOL}://${GIT_API_HOST}/git/$REPO_ID..."
        GIT_CLONE_URL="${GIT_API_PROTOCOL}://api:${USER_API_TOKEN}@${GIT_API_HOST}/git/${REPO_ID}"

        if git clone "$GIT_CLONE_URL" "$CLONE_DIR" 2>&1; then
            echo "  ✅ Successfully cloned to $CLONE_DIR"
        else
            echo "  ❌ Failed to clone $REPO_NAME"
            echo ""
            echo "  This could be caused by:"
            echo "    - Invalid repository credentials"
            echo "    - Repository doesn't exist"
            echo "    - Network connectivity issues"
            echo ""
        fi
    done

    echo "========================================="
    echo ""
else
    echo "========================================="
    echo "No repositories to clone"
    if [ -z "$HELIX_REPOSITORIES" ]; then
        echo "  HELIX_REPOSITORIES not set"
    fi
    if [ -z "$USER_API_TOKEN" ]; then
        echo "  USER_API_TOKEN not set"
    fi
    echo "========================================="
    echo ""
fi

# =========================================
# Setup helix-specs worktree
# =========================================
if [ -n "$HELIX_PRIMARY_REPO_NAME" ]; then
    echo "========================================="
    echo "Setting up design docs worktree..."
    echo "========================================="

    PRIMARY_REPO_PATH="$WORK_DIR/$HELIX_PRIMARY_REPO_NAME"

    if [ -d "$PRIMARY_REPO_PATH/.git" ]; then
        # helix-specs-create.sh was sourced at script start, use create_helix_specs_branch
        if type create_helix_specs_branch &>/dev/null; then
            create_helix_specs_branch "$PRIMARY_REPO_PATH"
            BRANCH_EXISTS="$HELIX_SPECS_BRANCH_EXISTS"
        else
            echo "  ⚠️  create_helix_specs_branch function not available"
            BRANCH_EXISTS=false
        fi

        if [ "$BRANCH_EXISTS" = false ]; then
            echo "  ⚠️  Skipping worktree setup (branch doesn't exist)"
        else
            WORKTREE_PATH="$WORK_DIR/helix-specs"

            if [ ! -d "$WORKTREE_PATH" ]; then
                echo "  📁 Creating design docs worktree at $WORKTREE_PATH..."
                if git -C "$PRIMARY_REPO_PATH" worktree add "$WORKTREE_PATH" helix-specs 2>&1; then
                    echo "  ✅ Design docs worktree ready at ~/work/helix-specs"
                    CURRENT_BRANCH=$(git -C "$WORKTREE_PATH" branch --show-current)
                    echo "  📍 Current branch: $CURRENT_BRANCH"
                else
                    echo "  ⚠️  Failed to create worktree"
                fi
            else
                echo "  ✅ Design docs worktree already exists"
                CURRENT_BRANCH=$(git -C "$WORKTREE_PATH" branch --show-current 2>/dev/null || echo "unknown")
                echo "  📍 Current branch: $CURRENT_BRANCH"
            fi
        fi
    else
        echo "  ⚠️  Primary repository not found at $PRIMARY_REPO_PATH"
        echo "  Check the clone output above for errors"
    fi
    echo ""
else
    echo "No primary repository specified (HELIX_PRIMARY_REPO_NAME not set)"
    echo ""
fi

# =========================================
# Additional Setup
# =========================================
echo "========================================="
echo "Additional setup..."
echo "========================================="

# Create Claude Code state symlink if needed
CLAUDE_STATE_DIR=$WORK_DIR/.claude-state
if command -v claude &> /dev/null; then
    mkdir -p $CLAUDE_STATE_DIR
    rm -rf ~/.claude
    ln -sf $CLAUDE_STATE_DIR ~/.claude
    echo "✅ Claude: ~/.claude → $CLAUDE_STATE_DIR"
fi

# Initialize workspace with README if empty
if [ ! -f "$WORK_DIR/README.md" ] && [ -z "$(ls -A "$WORK_DIR" 2>/dev/null | grep -v '^\.')" ]; then
    cat > "$WORK_DIR/README.md" << 'HEREDOC'
# Welcome to Your Helix External Agent

This is your autonomous development workspace. The AI agent running in this environment
can read and write files, run commands, and collaborate with you through the Helix interface.

## Getting Started

- This workspace is persistent across sessions
- Files you create here are saved automatically
- The AI agent has full access to this directory
- Use the Helix chat interface to direct the agent
HEREDOC
    echo "✅ Created README.md to initialize workspace"
fi

# Configure Zed keybindings for system clipboard
mkdir -p ~/.config/zed
cat > ~/.config/zed/keymap.json << 'KEYMAP_EOF'
[
  {
    "bindings": {
      "ctrl-c": "editor::Copy",
      "ctrl-v": "editor::Paste",
      "ctrl-x": "editor::Cut"
    }
  }
]
KEYMAP_EOF
echo "✅ Zed keymap configured for system clipboard"

# Configure SSH agent if keys exist
if [ -d "$HOME/.ssh" ] && [ "$(ls -A $HOME/.ssh/*.key 2>/dev/null)" ]; then
    echo "Setting up SSH agent for git access..."
    eval "$(ssh-agent -s)" > /dev/null
    for key in $HOME/.ssh/*.key; do
        ssh-add "$key" 2>/dev/null && echo "✅ Loaded SSH key: $(basename $key)"
    done
fi

echo ""

# =========================================
# Determine Zed folders and write to file
# =========================================
echo "========================================="
echo "Determining workspace folders..."
echo "========================================="

# Add primary repository first (if set and exists)
if [ -n "$HELIX_PRIMARY_REPO_NAME" ]; then
    PRIMARY_REPO_DIR="$WORK_DIR/$HELIX_PRIMARY_REPO_NAME"
    if [ -d "$PRIMARY_REPO_DIR" ]; then
        ZED_FOLDERS+=("$PRIMARY_REPO_DIR")
        echo "  📁 Primary: $HELIX_PRIMARY_REPO_NAME"
    fi
fi

# Add design docs worktree second (if exists)
DESIGN_DOCS_DIR="$WORK_DIR/helix-specs"
if [ -d "$DESIGN_DOCS_DIR" ]; then
    ZED_FOLDERS+=("$DESIGN_DOCS_DIR")
    echo "  📁 Design docs: helix-specs"
fi

# Add all other repositories (not primary, not internal)
if [ -n "$HELIX_REPOSITORIES" ]; then
    IFS=',' read -ra REPOS <<< "$HELIX_REPOSITORIES"
    for REPO_SPEC in "${REPOS[@]}"; do
        IFS=':' read -r REPO_ID REPO_NAME REPO_TYPE <<< "$REPO_SPEC"

        # Skip internal repos
        if [ "$REPO_TYPE" = "internal" ]; then
            continue
        fi

        # Skip primary repo (already added)
        if [ "$REPO_NAME" = "$HELIX_PRIMARY_REPO_NAME" ]; then
            continue
        fi

        # Add other code repos
        REPO_DIR="$WORK_DIR/$REPO_NAME"
        if [ -d "$REPO_DIR" ]; then
            ZED_FOLDERS+=("$REPO_DIR")
            echo "  📁 Other: $REPO_NAME"
        fi
    done
fi

# Fallback to work directory if no folders found
if [ ${#ZED_FOLDERS[@]} -eq 0 ]; then
    ZED_FOLDERS+=("$WORK_DIR")
    echo "  📁 Fallback: ~/work (no repositories cloned)"
fi

# Write folders to file for main script to read
printf '%s\n' "${ZED_FOLDERS[@]}" > "$FOLDERS_FILE"
echo ""
echo "Zed will open ${#ZED_FOLDERS[@]} folder(s)"
echo ""

# =========================================
# Run project startup script (if exists)
# =========================================
# Startup script is now in helix-specs branch (not main branch)
# This avoids modifying protected main branches on external repos
HELIX_SPECS_DIR="$WORK_DIR/helix-specs"
STARTUP_SCRIPT_PATH=""

if [ -d "$HELIX_SPECS_DIR" ]; then
    STARTUP_SCRIPT_PATH="$HELIX_SPECS_DIR/.helix/startup.sh"
fi

if [ -n "$STARTUP_SCRIPT_PATH" ] && [ -f "$STARTUP_SCRIPT_PATH" ]; then
    echo "========================================="
    echo "Running Project Startup Script"
    echo "Script: $STARTUP_SCRIPT_PATH"
    echo "========================================="
    echo ""

    # Change to primary repository for running commands
    if [ -n "$HELIX_PRIMARY_REPO_NAME" ]; then
        PRIMARY_REPO_PATH="$WORK_DIR/$HELIX_PRIMARY_REPO_NAME"
        if [ -d "$PRIMARY_REPO_PATH" ]; then
            cd "$PRIMARY_REPO_PATH"
            echo "📂 Working in: $HELIX_PRIMARY_REPO_NAME"
        fi
    fi
    echo ""

    # Run the startup script
    if bash -i "$STARTUP_SCRIPT_PATH"; then
        echo ""
        echo "========================================="
        echo "✅ Startup script completed successfully"
        echo "========================================="
    else
        EXIT_CODE=$?
        echo ""
        echo "========================================="
        echo "❌ Startup script failed with exit code $EXIT_CODE"
        echo "========================================="
        echo ""
        echo "💡 To fix this:"
        echo "   1. Edit the startup script in Project Settings"
        echo "   2. Click 'Test Startup Script' to test your changes"
    fi
    echo ""
else
    if [ -d "$HELIX_SPECS_DIR" ]; then
        echo "No startup script found at .helix/startup.sh in helix-specs"
        echo "Add a startup script in Project Settings to run setup commands"
    elif [ -n "$HELIX_PRIMARY_REPO_NAME" ]; then
        echo "No helix-specs worktree found"
        echo "Startup script should be in helix-specs/.helix/startup.sh"
    else
        echo "No startup script - no primary repository configured"
    fi
    echo ""
fi

# =========================================
# Signal completion to main script
# =========================================
touch "$COMPLETE_SIGNAL"

echo "========================================="
echo "✅ Setup complete! Zed is starting..."
echo "========================================="
echo ""
echo "This terminal will remain open for debugging."
echo "You can run commands here or close this window."
echo ""
echo "What would you like to do?"
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
SETUP_SCRIPT_EOF

chmod +x "$SETUP_SCRIPT"

echo "Launching setup terminal..."
echo "All setup work will be visible in the kitty terminal."
echo ""

# ALWAYS launch kitty with the setup script
# User can see all cloning, errors, and setup progress
kitty --title="Helix Workspace Setup" \
      --directory="$WORK_DIR" \
      bash "$SETUP_SCRIPT" &

KITTY_PID=$!
echo "Setup terminal launched (PID: $KITTY_PID)"

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
        echo "⚠️  Setup timeout after ${MAX_WAIT}s, proceeding anyway..."
        break
    fi
done

if [ -f "$COMPLETE_SIGNAL" ]; then
    echo "✅ Setup complete"
fi

# Wait for settings-sync-daemon to create Zed configuration
echo "Waiting for Zed configuration to be initialized..."
WAIT_COUNT=0
MAX_WAIT=30

while [ $WAIT_COUNT -lt $MAX_WAIT ]; do
    if [ -f "$HOME/.config/zed/settings.json" ]; then
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

# Fallback to current directory if no folders
if [ ${#ZED_FOLDERS[@]} -eq 0 ]; then
    ZED_FOLDERS=("$WORK_DIR")
    echo "Opening Zed in work directory (no folders from setup)"
else
    echo "Opening Zed with ${#ZED_FOLDERS[@]} folder(s):"
    for folder in "${ZED_FOLDERS[@]}"; do
        echo "  - $(basename "$folder")"
    done
fi

# Add Sway user guide as a separate file to open (if it exists)
USER_GUIDE_PATH="$WORK_DIR/SWAY-USER-GUIDE.md"
if [ -f "$USER_GUIDE_PATH" ]; then
    echo "  + SWAY-USER-GUIDE.md (opening as file)"
fi

# Launch ACP log viewer in Kitty (for debugging agent issues)
# This runs in background and provides visibility into Qwen Code/agent behavior
if [ "$SHOW_ACP_DEBUG_LOGS" = "true" ] || [ -n "$HELIX_DEBUG" ]; then
    echo "Starting ACP log viewer in Kitty terminal..."
    kitty --class acp-log-viewer \
          --title "ACP Agent Logs" \
          -e bash -c '
              echo "═══════════════════════════════════════════════════════════════"
              echo "  ACP Agent Log Viewer - Tailing Zed and Qwen Code logs"
              echo "═══════════════════════════════════════════════════════════════"
              echo ""
              echo "Waiting for Zed logs to appear..."
              echo ""
              # Wait for logs directory to exist
              while [ ! -d ~/.local/share/zed/logs ]; do
                  sleep 1
              done
              # Tail all log files - unfiltered for full visibility
              tail -F ~/.local/share/zed/logs/*.log 2>/dev/null
          ' &
    echo "ACP log viewer started in background"
fi

# Launch Zed in a restart loop
echo "Starting Zed with auto-restart loop (close window to reload updated binary)"
echo "Using Wayland backend (WAYLAND_DISPLAY=$WAYLAND_DISPLAY)"

while true; do
    # Open folders + user guide file (if exists)
    if [ -f "$USER_GUIDE_PATH" ]; then
        /zed-build/zed "${ZED_FOLDERS[@]}" "$USER_GUIDE_PATH" || true
    else
        /zed-build/zed "${ZED_FOLDERS[@]}" || true
    fi
    echo "Zed exited, restarting in 2 seconds..."
    sleep 2
done
