#!/bin/bash
# Startup script for Zed editor connected to Helix controlplane (Ubuntu GNOME version)
#
# Don't use set -e here - we need to handle failures gracefully
# set -e

# Redirect all output to log file AND stdout (using tee)
STARTUP_LOG="$HOME/.helix-startup.log"
exec > >(tee "$STARTUP_LOG") 2>&1

echo "========================================="
echo "Helix Agent Startup (Ubuntu GNOME) - $(date)"
echo "========================================="
echo ""

# Debug: Show key environment variables (sanitized)
if [ -n "$USER_API_TOKEN" ]; then
    echo "USER_API_TOKEN: ${USER_API_TOKEN:0:8}..."
else
    echo "USER_API_TOKEN: not set"
fi

if [ -n "$HELIX_REPOSITORIES" ]; then
    echo "HELIX_REPOSITORIES: $HELIX_REPOSITORIES"
else
    echo "HELIX_REPOSITORIES: not set"
fi

echo "HELIX_SESSION_ID: ${HELIX_SESSION_ID:-not set}"
echo "HELIX_API_BASE_URL: ${HELIX_API_BASE_URL:-not set}"
echo "HELIX_PRIMARY_REPO_NAME: ${HELIX_PRIMARY_REPO_NAME:-not set}"
echo ""

# Check if Zed binary exists (directory mounted to survive inode changes on rebuild)
if [ ! -f "/zed-build/zed" ]; then
    echo "Zed binary not found at /zed-build/zed - cannot start Zed agent"
    exit 1
fi

# Environment variables are passed from Wolf executor via container env
# HELIX_API_URL, HELIX_API_TOKEN, ANTHROPIC_API_KEY should be available

# NOTE: Zed state symlinks are already created by startup-app.sh BEFORE desktop starts
# This ensures settings-sync-daemon can write config.json immediately on startup

# Set workspace to mounted work directory
WORK_DIR="$HOME/work"
cd $WORK_DIR

# Configure git user identity FIRST (required for commits)
if [ -n "$GIT_USER_NAME" ]; then
    git config --global user.name "$GIT_USER_NAME"
    echo "Git user.name: $GIT_USER_NAME"
else
    # Default for Helix agents
    git config --global user.name "Helix Agent"
    echo "Git user.name: Helix Agent (default)"
fi

# CRITICAL: Enterprise ADO deployments reject commits from non-corporate email addresses
# The wolf_executor MUST always set GIT_USER_EMAIL - missing is a bug
if [ -n "$GIT_USER_EMAIL" ]; then
    git config --global user.email "$GIT_USER_EMAIL"
    echo "Git user.email: $GIT_USER_EMAIL"
else
    echo "FATAL: GIT_USER_EMAIL not set"
    echo "   Enterprise ADO deployments reject commits from non-corporate email addresses"
    echo "   This is a bug in wolf_executor - it should always pass GIT_USER_EMAIL"
    exit 1
fi

# Configure git to use merge commits (not rebase) for concurrent agent work
git config --global pull.rebase false
echo "Git pull strategy: merge (for concurrent agent compatibility)"

# Configure git credentials for HTTP operations (MUST happen before cloning!)
# Use user's API token for RBAC-enforced git operations
if [ -n "$USER_API_TOKEN" ] && [ -n "$HELIX_API_BASE_URL" ]; then
    # Set up git credential helper to use user's API token
    # Format: http://api:{user-token}@{helix-api-host}/git/{repo-id}
    # This ensures RBAC is enforced - agent can only access repos user has access to
    git config --global credential.helper 'store --file ~/.git-credentials'

    # Extract host from HELIX_API_BASE_URL (e.g., "http://example.helix.ml:8080" -> "example.helix.ml:8080")
    GIT_API_HOST=$(echo "$HELIX_API_BASE_URL" | sed 's|^https\?://||')
    GIT_API_PROTOCOL=$(echo "$HELIX_API_BASE_URL" | grep -o '^https\?' || echo "http")

    # Write credentials for the API host
    echo "${GIT_API_PROTOCOL}://api:${USER_API_TOKEN}@${GIT_API_HOST}" > ~/.git-credentials
    chmod 600 ~/.git-credentials

    echo "Git credentials configured for $GIT_API_HOST (user's API token for RBAC)"
else
    echo "Warning: USER_API_TOKEN or HELIX_API_BASE_URL not set - git operations will fail"
fi
echo ""

# Clone project repositories using Helix git HTTP server
# Repositories are cloned via HTTP with USER_API_TOKEN for RBAC enforcement
# Format: HELIX_REPOSITORIES="id:name:type,id:name:type,..."
# NOTE: Internal repos are no longer used - startup script lives in primary CODE repo
if [ -n "$HELIX_REPOSITORIES" ] && [ -n "$USER_API_TOKEN" ]; then
    echo "========================================="
    echo "Cloning project repositories..."
    echo "========================================="

    IFS=',' read -ra REPOS <<< "$HELIX_REPOSITORIES"
    for REPO_SPEC in "${REPOS[@]}"; do
        # Parse "id:name:type" format
        IFS=':' read -r REPO_ID REPO_NAME REPO_TYPE <<< "$REPO_SPEC"

        # Skip internal repos - they're deprecated
        # Startup script now lives in the primary CODE repo at .helix/startup.sh
        if [ "$REPO_TYPE" = "internal" ]; then
            echo "Skipping internal repo: $REPO_NAME (deprecated - startup script in code repo)"
            continue
        fi

        echo "Repository: $REPO_NAME (type: $REPO_TYPE)"
        CLONE_DIR="$WORK_DIR/$REPO_NAME"

        # If already cloned, just skip (startup script is in helix-specs worktree)
        if [ -d "$CLONE_DIR/.git" ]; then
            echo "  ✅ Already cloned at $CLONE_DIR"
            continue
        fi

        # Clone repository using HTTP with credentials in URL
        # Use HELIX_API_BASE_URL not hardcoded api:8080
        GIT_API_HOST=$(echo "$HELIX_API_BASE_URL" | sed 's|^https\?://||')
        GIT_API_PROTOCOL=$(echo "$HELIX_API_BASE_URL" | grep -o '^https\?' || echo "http")
        echo "  Cloning from ${GIT_API_PROTOCOL}://${GIT_API_HOST}/git/$REPO_ID..."
        GIT_CLONE_URL="${GIT_API_PROTOCOL}://api:${USER_API_TOKEN}@${GIT_API_HOST}/git/${REPO_ID}"

        if git clone "$GIT_CLONE_URL" "$CLONE_DIR" 2>&1; then
            echo "  Successfully cloned to $CLONE_DIR"
        else
            echo "  Failed to clone $REPO_NAME"
            # Don't exit - continue with other repos
        fi
    done

    echo "========================================="
    echo ""
fi

# Setup helix-specs worktree for primary repository
# This must happen INSIDE the Wolf container so git paths are correct for this environment
# The primary repository is cloned above in the repository cloning section
if [ -n "$HELIX_PRIMARY_REPO_NAME" ]; then
    echo "Setting up design docs worktree for primary repository: $HELIX_PRIMARY_REPO_NAME"

    PRIMARY_REPO_PATH="$WORK_DIR/$HELIX_PRIMARY_REPO_NAME"

    # Check if primary repository exists
    if [ -d "$PRIMARY_REPO_PATH/.git" ]; then
        # Source the helix-specs creation helper (handles edge cases like empty repos, detached HEAD, etc.)
        # Logic is in separate file so it can be tested independently
        SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
        if [ -f "$SCRIPT_DIR/helix-specs-create.sh" ]; then
            source "$SCRIPT_DIR/helix-specs-create.sh"

            # Create helix-specs branch if it doesn't exist
            # Sets HELIX_SPECS_BRANCH_EXISTS=true/false
            create_helix_specs_branch "$PRIMARY_REPO_PATH"
            BRANCH_EXISTS="$HELIX_SPECS_BRANCH_EXISTS"

            # Only create worktree if branch exists
            if [ "$BRANCH_EXISTS" = false ]; then
                echo "  Skipping worktree setup (branch doesn't exist)"
            else
                # Create worktree at top-level workspace for consistent path
                # Location: ~/work/helix-specs (consistent regardless of repo name)
                WORKTREE_PATH="$WORK_DIR/helix-specs"

                # Ensure path is absolute
                if [[ ! "$WORKTREE_PATH" = /* ]]; then
                    WORKTREE_PATH="$(cd "$WORK_DIR" && pwd)/helix-specs"
                fi

                if [ ! -d "$WORKTREE_PATH" ]; then
                    echo "  Creating design docs worktree at $WORKTREE_PATH..."
                    echo "  Running: git -C $PRIMARY_REPO_PATH worktree add $WORKTREE_PATH helix-specs"

                    if git -C "$PRIMARY_REPO_PATH" worktree add "$WORKTREE_PATH" helix-specs 2>&1; then
                        echo "  Design docs worktree ready at ~/work/helix-specs"

                        # Verify it's checked out at the right branch
                        CURRENT_BRANCH=$(git -C "$WORKTREE_PATH" branch --show-current)
                        echo "  Current branch: $CURRENT_BRANCH"
                    else
                        echo "  Failed to create worktree"
                    fi
                else
                    echo "  Design docs worktree already exists at ~/work/helix-specs"
                    CURRENT_BRANCH=$(git -C "$WORKTREE_PATH" branch --show-current 2>/dev/null || echo "unknown")
                    echo "  Current branch: $CURRENT_BRANCH"
                fi
            fi
        else
            echo "  helix-specs-create.sh not found at $SCRIPT_DIR, skipping worktree setup"
        fi
    else
        echo "  Primary repository not found at $PRIMARY_REPO_PATH"
        echo "  Repository should be cloned above in repository cloning section"
    fi
else
    echo "No primary repository specified (HELIX_PRIMARY_REPO_NAME not set)"
    echo "Skipping design docs worktree setup"
fi

# =========================================
# Install Helix Git Hooks
# =========================================
# Source the git hooks helper and install commit-msg hooks
# These automatically add Code-Ref and Spec-Ref trailers to commits
# Check /usr/local/bin first (production), then /helix-dev/shared (dev mode)
if [ -f "/usr/local/bin/helix-git-hooks.sh" ]; then
    source "/usr/local/bin/helix-git-hooks.sh"
    install_helix_git_hooks
elif [ -f "/helix-dev/shared/helix-git-hooks.sh" ]; then
    source "/helix-dev/shared/helix-git-hooks.sh"
    install_helix_git_hooks
else
    echo "helix-git-hooks.sh not found - git hooks not installed"
fi

# Create Claude Code state symlink if needed
CLAUDE_STATE_DIR=$WORK_DIR/.claude-state
if command -v claude &> /dev/null; then
    mkdir -p $CLAUDE_STATE_DIR
    rm -rf ~/.claude
    ln -sf $CLAUDE_STATE_DIR ~/.claude
    echo "Claude: ~/.claude -> $CLAUDE_STATE_DIR"
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

# Configure Zed keybindings to use system clipboard (X11 xclip)
# By default, Ctrl+C/V use Zed's internal clipboard
# We rebind to editor::Copy/Paste which sync with X11 system clipboard via xclip
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
echo "Zed keymap configured for system clipboard integration (X11/xclip)"

# Configure SSH agent and load keys for git access
if [ -d "$HOME/.ssh" ] && [ "$(ls -A $HOME/.ssh/*.key 2>/dev/null)" ]; then
    echo "Setting up SSH agent for git access..."
    eval "$(ssh-agent -s)"
    for key in $HOME/.ssh/*.key; do
        ssh-add "$key" 2>/dev/null && echo "Loaded SSH key: $(basename $key)"
    done
fi

# Git user and credentials already configured above (before repository cloning)

# Execute project startup script from helix-specs worktree
# Startup script lives at .helix/startup.sh in the helix-specs branch
# This avoids modifying protected main branches on external repos

HELIX_SPECS_DIR="$WORK_DIR/helix-specs"
STARTUP_SCRIPT_PATH=""

if [ -d "$HELIX_SPECS_DIR" ]; then
    STARTUP_SCRIPT_PATH="$HELIX_SPECS_DIR/.helix/startup.sh"
    echo "========================================="
    echo "Looking for startup script in helix-specs..."
    echo "Script path: $STARTUP_SCRIPT_PATH"
    echo "========================================="
else
    echo "No helix-specs worktree found"
    echo "Startup script should be in helix-specs/.helix/startup.sh"
fi

if [ -n "$STARTUP_SCRIPT_PATH" ] && [ -f "$STARTUP_SCRIPT_PATH" ]; then
    echo "========================================="
    echo "Found project startup script"
    echo "Script: $STARTUP_SCRIPT_PATH"
    echo "========================================="

    # Create wrapper script that runs the startup script and handles errors
    WRAPPER_SCRIPT="$WORK_DIR/.helix-startup-wrapper.sh"
    cat > "$WRAPPER_SCRIPT" <<WRAPPER_EOF
#!/bin/bash

# Show main startup log first so user can scroll up and see what happened
STARTUP_LOG="\$HOME/.helix-startup.log"
if [ -f "\$STARTUP_LOG" ]; then
    echo "========================================="
    echo "Helix Agent Startup Log"
    echo "========================================="
    cat "\$STARTUP_LOG"
    echo ""
    echo "========================================="
    echo ""
fi

echo "========================================="
echo "Running Project Startup Script"
echo "Script: $STARTUP_SCRIPT_PATH"
echo "Working Directory: ${HELIX_PRIMARY_REPO_NAME:-~/work} (primary repository)"
echo "========================================="
echo ""

# Change to primary repository before running startup script
# The script should run in the context of the code repository, not design docs
if [ -n "$HELIX_PRIMARY_REPO_NAME" ]; then
    PRIMARY_REPO_DIR="$WORK_DIR/$HELIX_PRIMARY_REPO_NAME"
    if [ -d "\$PRIMARY_REPO_DIR" ]; then
        cd "\$PRIMARY_REPO_DIR"
        echo "Working in: $HELIX_PRIMARY_REPO_NAME"
    else
        cd "$WORK_DIR"
        echo "Working in: ~/work (primary repo not found)"
    fi
else
    cd "$WORK_DIR"
    echo "Working in: ~/work"
fi
echo ""

# Run the startup script in interactive mode (no timeout)
# Interactive mode allows apt progress bars to work properly in the terminal
if bash -i "$STARTUP_SCRIPT_PATH"; then
    echo ""
    echo "========================================="
    echo "Startup script completed successfully"
    echo "========================================="
else
    EXIT_CODE=\$?
    echo ""
    echo "========================================="
    echo "Startup script failed with exit code \$EXIT_CODE"
    echo "========================================="
    echo ""
    echo "To fix this:"
    echo "   1. Edit the startup script in Project Settings"
    echo "   2. Click 'Test Startup Script' to test your changes"
    echo "   3. Iterate until it works, then save"
fi

echo ""
echo "What would you like to do?"
echo "  1) Close this window"
echo "  2) Start an interactive shell"
echo ""
read -p "Enter choice [1-2]: " choice

case "\$choice" in
    1)
        echo "Closing..."
        exit 0
        ;;
    2)
        echo ""
        echo "Starting interactive shell..."
        echo "Type 'exit' to close this window."
        echo ""
        if [ -n "$HELIX_PRIMARY_REPO_NAME" ]; then
            PRIMARY_REPO_DIR="$WORK_DIR/$HELIX_PRIMARY_REPO_NAME"
            if [ -d "\$PRIMARY_REPO_DIR" ]; then
                cd "\$PRIMARY_REPO_DIR"
            else
                cd "$WORK_DIR"
            fi
        else
            cd "$WORK_DIR"
        fi
        exec bash
        ;;
    *)
        echo "Invalid choice. Starting interactive shell..."
        cd "$WORK_DIR"
        exec bash
        ;;
esac
WRAPPER_EOF
    chmod +x "$WRAPPER_SCRIPT"

    # Launch terminal in background to run the wrapper script
    # Use gnome-terminal for Ubuntu GNOME desktop
    gnome-terminal --title="Project Startup Script" \
            --working-directory="$WORK_DIR" \
            -- bash "$WRAPPER_SCRIPT" &

    echo "Startup script terminal launched (check for new terminal window)"
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
            echo "Zed configuration ready with default_model"
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

# Trap signals to prevent script exit when Zed is closed
# Using signal numbers for compatibility: 15=TERM, 2=INT, 1=HUP
trap 'echo "Caught signal, continuing restart loop..."' 15 2 1

# Clear Zed's workspace state to prevent fullscreen mode restoration
# Zed remembers fullscreen state in its workspace database
# Deleting this file forces Zed to start in normal windowed mode with decorations
if [ -f "$HOME/.local/share/zed/db/0-stable.db" ]; then
    echo "Clearing Zed workspace state to prevent fullscreen mode..."
    rm -f "$HOME/.local/share/zed/db/0-stable.db"
fi

# Note: No HiDPI scaling - using vanilla Ubuntu with native resolution
# If you need scaling, configure it in GNOME Settings > Displays

# Determine which folders to open in Zed
# Open ALL repositories as multi-folder workspace: primary, design docs, then other repos
ZED_FOLDERS=()

# Add primary repository first (if set)
if [ -n "$HELIX_PRIMARY_REPO_NAME" ]; then
    PRIMARY_REPO_DIR="$WORK_DIR/$HELIX_PRIMARY_REPO_NAME"
    if [ -d "$PRIMARY_REPO_DIR" ]; then
        ZED_FOLDERS+=("$PRIMARY_REPO_DIR")
    fi
fi

# Add design docs worktree second (if exists)
DESIGN_DOCS_DIR="$WORK_DIR/helix-specs"
if [ -d "$DESIGN_DOCS_DIR" ]; then
    ZED_FOLDERS+=("$DESIGN_DOCS_DIR")
fi

# Add all other repositories (not primary, not internal)
if [ -n "$HELIX_REPOSITORIES" ]; then
    IFS=',' read -ra REPOS <<< "$HELIX_REPOSITORIES"
    for REPO_SPEC in "${REPOS[@]}"; do
        IFS=':' read -r REPO_ID REPO_NAME REPO_TYPE <<< "$REPO_SPEC"

        # Skip internal repos (config only, not code)
        if [ "$REPO_TYPE" = "internal" ]; then
            continue
        fi

        # Skip primary repo (already added first)
        if [ "$REPO_NAME" = "$HELIX_PRIMARY_REPO_NAME" ]; then
            continue
        fi

        # Add other code repos
        REPO_DIR="$WORK_DIR/$REPO_NAME"
        if [ -d "$REPO_DIR" ]; then
            ZED_FOLDERS+=("$REPO_DIR")
        fi
    done
fi

# Fallback to current directory if no folders found
if [ ${#ZED_FOLDERS[@]} -eq 0 ]; then
    ZED_FOLDERS=(".")
    echo "Opening Zed in current directory (no repositories configured)"
else
    echo "Opening Zed with multi-folder workspace (${#ZED_FOLDERS[@]} folders):"
    for folder in "${ZED_FOLDERS[@]}"; do
        echo "  - $(basename "$folder")"
    done
fi

# Launch ACP log viewer in gnome-terminal (for debugging agent issues)
# This runs in background and provides visibility into Qwen Code/agent behavior
if [ "$SHOW_ACP_DEBUG_LOGS" = "true" ] || [ -n "$HELIX_DEBUG" ]; then
    echo "Starting ACP log viewer in gnome-terminal..."
    gnome-terminal --class=acp-log-viewer \
          --title="ACP Agent Logs" \
          -- bash -c '
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

# Launch Zed in a restart loop for development
# When you close Zed (click X), it auto-restarts with the latest binary
# Perfect for testing rebuilds without recreating the entire container
echo "Starting Zed with auto-restart loop (close window to reload updated binary)"
echo "Using vanilla Ubuntu settings (no custom HiDPI scaling)"

# =========================================================================
# Performance tuning for Zed on XWayland
# =========================================================================
# Zed is laggy on XWayland due to the extra compositing layer and frame timing
# issues. These settings help mitigate the lag.
#
# For best performance, Zed should use native Wayland, but GNOME runs as an
# X11 session on XWayland in this container, so no Wayland socket is available.
# =========================================================================

# Disable vsync - let the timer-based refresh control frame pacing instead
# With XWayland, vsync (FIFO mode) can cause frame timing conflicts with
# the X11 refresh timer, leading to stuttering. MAILBOX mode is actually
# smoother on XWayland despite higher CPU usage.
# export ZED_DISPLAY_SYNC=block  # Disabled - causes more stutter on XWayland
echo "ZED_DISPLAY_SYNC=<default> (using MAILBOX mode for XWayland compatibility)"

# Disable MSAA for path rendering to reduce GPU/CPU overhead
# Sample count of 1 means no anti-aliasing on paths, which improves performance
export ZED_PATH_SAMPLE_COUNT=1
echo "ZED_PATH_SAMPLE_COUNT=1 (MSAA disabled for performance)"

# NVIDIA-specific: Force threaded optimizations for better multi-core usage
export __GL_THREADED_OPTIMIZATIONS=1

# Limit Vulkan frame latency to reduce input lag
export VK_LAYER_NV_optimus_present_mode_hint=MAILBOX

while true; do
    echo "Launching Zed..."
    # Launch Zed directly - this blocks until Zed exits
    # GNOME icon matching works because:
    # 1. Zed sets app_id to "dev.zed.Zed-Dev" (from ReleaseChannel::Dev)
    # 2. Desktop file at /usr/share/applications/dev.zed.Zed-Dev.desktop has matching StartupWMClass
    # NOTE: gio launch doesn't block, so we can't use it in a restart loop
    /zed-build/zed "${ZED_FOLDERS[@]}" || true
    echo "Zed exited, restarting in 2 seconds..."
    sleep 2
done
