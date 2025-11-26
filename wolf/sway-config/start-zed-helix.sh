#!/bin/bash
# Startup script for Zed editor connected to Helix controlplane (Sway version)
set -e

# Redirect all output to log file AND stdout (using tee)
STARTUP_LOG="$HOME/.helix-startup.log"
exec > >(tee "$STARTUP_LOG") 2>&1

echo "========================================="
echo "Helix Agent Startup - $(date)"
echo "========================================="
echo ""

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
echo ""

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
WORK_DIR="$HOME/work"
cd $WORK_DIR

# Configure git user identity FIRST (required for commits)
if [ -n "$GIT_USER_NAME" ]; then
    git config --global user.name "$GIT_USER_NAME"
    echo "✅ Git user.name: $GIT_USER_NAME"
else
    # Default for Helix agents
    git config --global user.name "Helix Agent"
    echo "✅ Git user.name: Helix Agent (default)"
fi

if [ -n "$GIT_USER_EMAIL" ]; then
    git config --global user.email "$GIT_USER_EMAIL"
    echo "✅ Git user.email: $GIT_USER_EMAIL"
else
    # Default for Helix agents
    git config --global user.email "agent@helix.ml"
    echo "✅ Git user.email: agent@helix.ml (default)"
fi

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

    echo "✅ Git credentials configured for $GIT_API_HOST (user's API token for RBAC)"
else
    echo "⚠️  USER_API_TOKEN or HELIX_API_BASE_URL not set - git operations will fail"
fi
echo ""

# Clone project repositories using Helix git HTTP server
# Repositories are cloned via HTTP with USER_API_TOKEN for RBAC enforcement
# Format: HELIX_REPOSITORIES="id:name:type,id:name:type,..."
if [ -n "$HELIX_REPOSITORIES" ] && [ -n "$USER_API_TOKEN" ]; then
    echo "========================================="
    echo "Cloning project repositories..."
    echo "========================================="

    IFS=',' read -ra REPOS <<< "$HELIX_REPOSITORIES"
    for REPO_SPEC in "${REPOS[@]}"; do
        # Parse "id:name:type" format
        IFS=':' read -r REPO_ID REPO_NAME REPO_TYPE <<< "$REPO_SPEC"

        echo "📦 Repository: $REPO_NAME (type: $REPO_TYPE)"

        # Determine clone directory based on repo type
        if [ "$REPO_TYPE" = "internal" ]; then
            CLONE_DIR="$WORK_DIR/.helix-project"
        else
            CLONE_DIR="$WORK_DIR/$REPO_NAME"
        fi

        # If already cloned, skip (preserve agent's work)
        # This is important for SpecTask workflow: planning → implementation reuses same workspace
        if [ -d "$CLONE_DIR/.git" ]; then
            echo "  ✅ Already cloned at $CLONE_DIR (skipping)"
            continue
        fi

        # Clone repository using HTTP with credentials in URL
        # Use HELIX_API_BASE_URL not hardcoded api:8080
        GIT_API_HOST=$(echo "$HELIX_API_BASE_URL" | sed 's|^https\?://||')
        GIT_API_PROTOCOL=$(echo "$HELIX_API_BASE_URL" | grep -o '^https\?' || echo "http")
        echo "  📥 Cloning from ${GIT_API_PROTOCOL}://${GIT_API_HOST}/git/$REPO_ID..."
        GIT_CLONE_URL="${GIT_API_PROTOCOL}://api:${USER_API_TOKEN}@${GIT_API_HOST}/git/${REPO_ID}"

        if git clone "$GIT_CLONE_URL" "$CLONE_DIR" 2>&1; then
            echo "  ✅ Successfully cloned to $CLONE_DIR"
        else
            echo "  ❌ Failed to clone $REPO_NAME"
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
        source "$SCRIPT_DIR/helix-specs-create.sh"

        # Create helix-specs branch if it doesn't exist
        # Sets HELIX_SPECS_BRANCH_EXISTS=true/false
        create_helix_specs_branch "$PRIMARY_REPO_PATH"
        BRANCH_EXISTS="$HELIX_SPECS_BRANCH_EXISTS"

        # Only create worktree if branch exists
        if [ "$BRANCH_EXISTS" = false ]; then
            echo "  ⚠️  Skipping worktree setup (branch doesn't exist)"
        else
            # Create worktree at top-level workspace for consistent path
            # Location: ~/work/helix-specs (consistent regardless of repo name)
            WORKTREE_PATH="$WORK_DIR/helix-specs"

            # Ensure path is absolute
            if [[ ! "$WORKTREE_PATH" = /* ]]; then
                WORKTREE_PATH="$(cd "$WORK_DIR" && pwd)/helix-specs"
            fi

            if [ ! -d "$WORKTREE_PATH" ]; then
                echo "  📁 Creating design docs worktree at $WORKTREE_PATH..."
                echo "  Running: git -C $PRIMARY_REPO_PATH worktree add $WORKTREE_PATH helix-specs"

                if git -C "$PRIMARY_REPO_PATH" worktree add "$WORKTREE_PATH" helix-specs 2>&1; then
                    echo "  ✅ Design docs worktree ready at ~/work/helix-specs"

                    # Verify it's checked out at the right branch
                    CURRENT_BRANCH=$(git -C "$WORKTREE_PATH" branch --show-current)
                    echo "  📍 Current branch: $CURRENT_BRANCH"
                else
                    echo "  ⚠️  Failed to create worktree"
                fi
            else
                echo "  ✅ Design docs worktree already exists at ~/work/helix-specs"
                CURRENT_BRANCH=$(git -C "$WORKTREE_PATH" branch --show-current 2>/dev/null || echo "unknown")
                echo "  📍 Current branch: $CURRENT_BRANCH"
            fi
        fi
    else
        echo "  ⚠️  Primary repository not found at $PRIMARY_REPO_PATH"
        echo "  Repository should be cloned above in repository cloning section"
    fi
else
    echo "No primary repository specified (HELIX_PRIMARY_REPO_NAME not set)"
    echo "Skipping design docs worktree setup"
fi

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

# Configure Zed keybindings to use system clipboard (Wayland wl-clipboard)
# By default, Ctrl+C/V use Zed's internal clipboard
# We rebind to editor::Copy/Paste which sync with Wayland system clipboard
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
echo "✅ Zed keymap configured for system clipboard integration"

# Configure SSH agent and load keys for git access
if [ -d "$HOME/.ssh" ] && [ "$(ls -A $HOME/.ssh/*.key 2>/dev/null)" ]; then
    echo "Setting up SSH agent for git access..."
    eval "$(ssh-agent -s)"
    for key in $HOME/.ssh/*.key; do
        ssh-add "$key" 2>/dev/null && echo "Loaded SSH key: $(basename $key)"
    done
fi

# Git user and credentials already configured above (before repository cloning)

# Execute project startup script from internal Git repo - run in terminal window
# Internal repos are cloned directly to .helix-project (no guessing needed!)
INTERNAL_REPO_PATH="$WORK_DIR/.helix-project"

# Pull latest changes from internal repo before running startup script
# This ensures we have the latest version if user edited it in the UI
if [ -d "$INTERNAL_REPO_PATH/.git" ]; then
    echo "========================================="
    echo "Checking for startup script updates..."
    echo "========================================="

    # Detect the default branch (could be main or master)
    DEFAULT_BRANCH=$(git -C "$INTERNAL_REPO_PATH" symbolic-ref refs/remotes/origin/HEAD 2>/dev/null | sed 's@^refs/remotes/origin/@@')
    if [ -z "$DEFAULT_BRANCH" ]; then
        # Fallback: try main first, then master
        if git -C "$INTERNAL_REPO_PATH" show-ref --verify refs/remotes/origin/main >/dev/null 2>&1; then
            DEFAULT_BRANCH="main"
        elif git -C "$INTERNAL_REPO_PATH" show-ref --verify refs/remotes/origin/master >/dev/null 2>&1; then
            DEFAULT_BRANCH="master"
        else
            echo "⚠️  Could not detect default branch, skipping pull"
            DEFAULT_BRANCH=""
        fi
    fi

    if [ -n "$DEFAULT_BRANCH" ]; then
        if git -C "$INTERNAL_REPO_PATH" pull origin "$DEFAULT_BRANCH" 2>&1; then
            echo "✅ Internal repo up to date (branch: $DEFAULT_BRANCH)"
        else
            echo "⚠️  Git pull failed (may have local changes or network issue)"
        fi
    fi
    echo ""
fi

STARTUP_SCRIPT_PATH="$INTERNAL_REPO_PATH/.helix/startup.sh"

if [ -f "$STARTUP_SCRIPT_PATH" ]; then
    echo "========================================="
    echo "Found project startup script in Git repo"
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
echo "Running Project Startup Script from Git"
echo "Script: $STARTUP_SCRIPT_PATH"
echo "Working Directory: $HELIX_PRIMARY_REPO_NAME (primary repository)"
echo "========================================="
echo ""

# Change to primary repository before running startup script
# The script should run in the context of the code repository, not design docs
PRIMARY_REPO_DIR="$WORK_DIR/$HELIX_PRIMARY_REPO_NAME"
if [ -d "\$PRIMARY_REPO_DIR" ]; then
    cd "\$PRIMARY_REPO_DIR"
    echo "📂 Working in: $HELIX_PRIMARY_REPO_NAME"
else
    cd "$WORK_DIR"
    echo "📂 Working in: ~/work (primary repo not found)"
fi
echo ""

# Run the startup script in interactive mode (no timeout)
# Interactive mode allows apt progress bars to work properly in the terminal
if bash -i "$STARTUP_SCRIPT_PATH"; then
    echo ""
    echo "========================================="
    echo "✅ Startup script completed successfully"
    echo "========================================="
else
    EXIT_CODE=\$?
    echo ""
    echo "========================================="
    echo "❌ Startup script failed with exit code \$EXIT_CODE"
    echo "========================================="
    echo ""
    echo "💡 To fix this:"
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
        echo "Starting interactive shell in primary repository..."
        echo "Type 'exit' to close this window."
        echo ""
        PRIMARY_REPO_DIR="$WORK_DIR/$HELIX_PRIMARY_REPO_NAME"
        if [ -d "\$PRIMARY_REPO_DIR" ]; then
            cd "\$PRIMARY_REPO_DIR"
        else
            cd "$WORK_DIR"
        fi
        exec bash
        ;;
    *)
        echo "Invalid choice. Starting interactive shell..."
        PRIMARY_REPO_DIR="$WORK_DIR/$HELIX_PRIMARY_REPO_NAME"
        if [ -d "\$PRIMARY_REPO_DIR" ]; then
            cd "\$PRIMARY_REPO_DIR"
        else
            cd "$WORK_DIR"
        fi
        exec bash
        ;;
esac
WRAPPER_EOF
    chmod +x "$WRAPPER_SCRIPT"

    # Launch terminal in background to run the wrapper script
    # Use kitty terminal emulator (ghostty has OpenGL permission issues)
    # kitty: command goes at end without -e flag
    kitty --title="Project Startup Script" \
            --directory="$WORK_DIR" \
            bash "$WRAPPER_SCRIPT" &

    echo "Startup script terminal launched (check right side of screen)"
else
    echo "No startup script found - .helix-project/.helix/startup.sh doesn't exist"
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

while true; do
    /zed-build/zed "${ZED_FOLDERS[@]}" || true
    echo "Zed exited, restarting in 2 seconds..."
    sleep 2
done
