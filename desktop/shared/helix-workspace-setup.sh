#!/bin/bash
#
# helix-workspace-setup.sh - Common workspace setup for Helix desktop containers
#
# This script handles all setup work before Zed launches:
# - Git configuration
# - Repository cloning
# - Branch checkout
# - helix-specs worktree setup
# - Git hooks installation
# - Zed keymap configuration
#
# Usage: This script is run inside a terminal (kitty or gnome-terminal)
# so the user can see all output. It writes the Zed folders to a signal
# file when complete.
#
# Signal files:
#   $HOME/.helix-setup-complete  - Touched when setup is done
#   $HOME/.helix-zed-folders     - List of folders for Zed to open

# Exit on error - but trap will catch it and keep terminal open
set -e

# Trap any exit (success or failure) to show interactive menu
# This ensures users can see errors and debug
cleanup_and_prompt() {
    local exit_code=$?
    echo ""
    if [ $exit_code -ne 0 ]; then
        echo "========================================="
        echo "❌ Setup failed with exit code $exit_code"
        echo "========================================="
        echo ""
        echo "Check the errors above."
    fi

    echo ""
    echo "What would you like to do?"
    echo "  1) Close this window"
    echo "  2) Start an interactive shell for debugging"
    echo ""
    read -p "Enter choice [1-2]: " choice

    case "$choice" in
        1)
            # Disable trap before exiting to avoid infinite loop
            trap - EXIT
            exit $exit_code
            ;;
        2|*)
            echo ""
            echo "Starting interactive shell..."
            echo "Type 'exit' to close this window."
            echo ""
            if [ -n "$HELIX_PRIMARY_REPO_NAME" ] && [ -d "$HOME/work/$HELIX_PRIMARY_REPO_NAME" ]; then
                cd "$HOME/work/$HELIX_PRIMARY_REPO_NAME"
            else
                cd "$HOME/work"
            fi
            exec bash
            ;;
    esac
}
trap cleanup_and_prompt EXIT

echo "========================================="
echo "Helix Workspace Setup - $(date)"
echo "========================================="
echo ""

# Source the git helper functions (provides get_default_branch, create_helix_specs_branch)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [ -f "$SCRIPT_DIR/helix-specs-create.sh" ]; then
    source "$SCRIPT_DIR/helix-specs-create.sh"
elif [ -f "/usr/local/bin/helix-specs-create.sh" ]; then
    source "/usr/local/bin/helix-specs-create.sh"
else
    echo "Warning: helix-specs-create.sh not found - using fallback branch detection"
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

# Define paths
WORK_DIR="$HOME/work"
COMPLETE_SIGNAL="$HOME/.helix-setup-complete"
FOLDERS_FILE="$HOME/.helix-zed-folders"

# Clean up old signal files from previous runs
rm -f "$COMPLETE_SIGNAL" "$FOLDERS_FILE"

# Ensure work directory exists
mkdir -p "$WORK_DIR"
cd "$WORK_DIR"

# Track which folders should be opened in Zed
declare -a ZED_FOLDERS

# Debug: Show key environment variables (sanitized)
echo "Environment:"
if [ -n "$USER_API_TOKEN" ]; then
    echo "  USER_API_TOKEN: ${USER_API_TOKEN:0:8}..."
else
    echo "  USER_API_TOKEN: not set"
fi

if [ -n "$HELIX_REPOSITORIES" ]; then
    echo "  HELIX_REPOSITORIES: $HELIX_REPOSITORIES"
else
    echo "  HELIX_REPOSITORIES: not set"
fi

if [ -n "$HELIX_PRIMARY_REPO_NAME" ]; then
    echo "  HELIX_PRIMARY_REPO_NAME: $HELIX_PRIMARY_REPO_NAME"
else
    echo "  HELIX_PRIMARY_REPO_NAME: not set"
fi
echo ""

# =========================================
# Git Configuration
# =========================================
echo "========================================="
echo "Configuring Git..."
echo "========================================="

# Configure git user identity (required for commits)
if [ -n "$GIT_USER_NAME" ]; then
    git config --global user.name "$GIT_USER_NAME"
    echo "  Git user.name: $GIT_USER_NAME"
else
    git config --global user.name "Helix Agent"
    echo "  Git user.name: Helix Agent (default)"
fi

# CRITICAL: Enterprise ADO deployments reject commits from non-corporate email addresses
if [ -n "$GIT_USER_EMAIL" ]; then
    git config --global user.email "$GIT_USER_EMAIL"
    echo "  Git user.email: $GIT_USER_EMAIL"
else
    echo "  ⚠️  WARNING: GIT_USER_EMAIL not set"
    echo "  Enterprise ADO deployments may reject commits from non-corporate email addresses"
    echo "  Using fallback email: agent@helix.ml"
    git config --global user.email "agent@helix.ml"
fi

# Configure git to use merge commits (not rebase) for concurrent agent work
git config --global pull.rebase false
echo "  Git pull strategy: merge"

# Configure git credentials for HTTP operations (MUST happen before cloning!)
if [ -n "$USER_API_TOKEN" ] && [ -n "$HELIX_API_BASE_URL" ]; then
    git config --global credential.helper 'store --file ~/.git-credentials'
    GIT_API_HOST=$(echo "$HELIX_API_BASE_URL" | sed 's|^https\?://||')
    GIT_API_PROTOCOL=$(echo "$HELIX_API_BASE_URL" | grep -o '^https\?' || echo "http")
    echo "${GIT_API_PROTOCOL}://api:${USER_API_TOKEN}@${GIT_API_HOST}" > ~/.git-credentials
    chmod 600 ~/.git-credentials
    echo "  Git credentials configured for $GIT_API_HOST"
else
    echo "  Warning: USER_API_TOKEN or HELIX_API_BASE_URL not set - git operations may fail"
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
            echo "  Skipping internal repo: $REPO_NAME (deprecated)"
            continue
        fi

        echo "  Repository: $REPO_NAME (type: $REPO_TYPE)"
        CLONE_DIR="$WORK_DIR/$REPO_NAME"

        # If already cloned, just skip
        if [ -d "$CLONE_DIR/.git" ]; then
            echo "    Already cloned at $CLONE_DIR"
            continue
        fi

        # Clone repository using HTTP with credentials
        GIT_API_HOST=$(echo "$HELIX_API_BASE_URL" | sed 's|^https\?://||')
        GIT_API_PROTOCOL=$(echo "$HELIX_API_BASE_URL" | grep -o '^https\?' || echo "http")
        echo "    Cloning from ${GIT_API_PROTOCOL}://${GIT_API_HOST}/git/$REPO_ID..."
        GIT_CLONE_URL="${GIT_API_PROTOCOL}://api:${USER_API_TOKEN}@${GIT_API_HOST}/git/${REPO_ID}"

        if ! git clone "$GIT_CLONE_URL" "$CLONE_DIR" 2>&1; then
            echo ""
            echo "    ❌ FAILED to clone $REPO_NAME"
            echo ""
            echo "    This could be caused by:"
            echo "      - Invalid repository credentials"
            echo "      - Repository doesn't exist"
            echo "      - Network connectivity issues"
            echo ""
            echo "    The terminal will stay open so you can see this error."
            exit 1
        fi
        echo "    ✅ Successfully cloned to $CLONE_DIR"
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
# Checkout correct branch (based on branch mode)
# =========================================
if [ -n "$HELIX_PRIMARY_REPO_NAME" ] && [ -n "$HELIX_BRANCH_MODE" ]; then
    echo "========================================="
    echo "Configuring branch..."
    echo "========================================="

    PRIMARY_REPO_PATH="$WORK_DIR/$HELIX_PRIMARY_REPO_NAME"

    if [ -d "$PRIMARY_REPO_PATH/.git" ]; then
        cd "$PRIMARY_REPO_PATH"

        # Fetch all remote branches to ensure we have the latest
        echo "  Fetching remote branches..."
        git fetch origin --prune 2>&1 || echo "  Warning: Failed to fetch (continuing anyway)"

        if [ "$HELIX_BRANCH_MODE" = "existing" ]; then
            # Existing branch mode: checkout the working branch
            if [ -n "$HELIX_WORKING_BRANCH" ]; then
                echo "  Mode: Continue existing branch"
                echo "  Checking out branch: $HELIX_WORKING_BRANCH"

                if git show-ref --verify --quiet "refs/heads/$HELIX_WORKING_BRANCH"; then
                    git checkout "$HELIX_WORKING_BRANCH" 2>&1
                    echo "  Checked out existing local branch: $HELIX_WORKING_BRANCH"
                elif git show-ref --verify --quiet "refs/remotes/origin/$HELIX_WORKING_BRANCH"; then
                    git checkout -b "$HELIX_WORKING_BRANCH" "origin/$HELIX_WORKING_BRANCH" 2>&1
                    echo "  Created tracking branch from origin: $HELIX_WORKING_BRANCH"
                else
                    echo "  Branch not found locally or remotely: $HELIX_WORKING_BRANCH"
                    echo "  Available remote branches:"
                    git branch -r | head -10
                fi
            else
                echo "  Warning: Existing mode but HELIX_WORKING_BRANCH not set"
            fi
        elif [ "$HELIX_BRANCH_MODE" = "new" ]; then
            # New branch mode: create branch from base
            echo "  Mode: Create new branch"

            # Determine base branch
            BASE_BRANCH="$HELIX_BASE_BRANCH"
            if [ -z "$BASE_BRANCH" ]; then
                BASE_BRANCH=$(git symbolic-ref refs/remotes/origin/HEAD 2>/dev/null | sed 's@^refs/remotes/origin/@@')
                if [ -z "$BASE_BRANCH" ]; then
                    if git show-ref --verify refs/remotes/origin/main >/dev/null 2>&1; then
                        BASE_BRANCH="main"
                    elif git show-ref --verify refs/remotes/origin/master >/dev/null 2>&1; then
                        BASE_BRANCH="master"
                    else
                        BASE_BRANCH="main"
                    fi
                fi
                echo "  Using detected default branch: $BASE_BRANCH"
            else
                echo "  Using specified base branch: $BASE_BRANCH"
            fi

            # Checkout base branch first
            if git show-ref --verify --quiet "refs/remotes/origin/$BASE_BRANCH"; then
                git checkout "$BASE_BRANCH" 2>&1 || git checkout -b "$BASE_BRANCH" "origin/$BASE_BRANCH" 2>&1
                git pull origin "$BASE_BRANCH" 2>&1 || echo "  Warning: Failed to pull (continuing anyway)"
            else
                echo "  Warning: Base branch not found: $BASE_BRANCH"
            fi

            # Create new working branch if specified
            if [ -n "$HELIX_WORKING_BRANCH" ]; then
                echo "  Creating new branch: $HELIX_WORKING_BRANCH from $BASE_BRANCH"

                if git show-ref --verify --quiet "refs/heads/$HELIX_WORKING_BRANCH"; then
                    echo "  Branch already exists locally, checking out: $HELIX_WORKING_BRANCH"
                    git checkout "$HELIX_WORKING_BRANCH" 2>&1
                elif git show-ref --verify --quiet "refs/remotes/origin/$HELIX_WORKING_BRANCH"; then
                    echo "  Branch already exists remotely, creating tracking branch: $HELIX_WORKING_BRANCH"
                    git checkout -b "$HELIX_WORKING_BRANCH" "origin/$HELIX_WORKING_BRANCH" 2>&1
                else
                    git checkout -b "$HELIX_WORKING_BRANCH" 2>&1
                    echo "  Created new branch: $HELIX_WORKING_BRANCH"
                fi
            fi
        fi

        CURRENT_BRANCH=$(git branch --show-current 2>/dev/null || echo "unknown")
        echo "  Current branch: $CURRENT_BRANCH"
        cd "$WORK_DIR"
    else
        echo "  Warning: Primary repository not found at $PRIMARY_REPO_PATH"
    fi
    echo ""
else
    if [ -z "$HELIX_BRANCH_MODE" ]; then
        echo "No branch mode specified - staying on default branch"
    fi
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
        if type create_helix_specs_branch &>/dev/null; then
            create_helix_specs_branch "$PRIMARY_REPO_PATH"
            BRANCH_EXISTS="$HELIX_SPECS_BRANCH_EXISTS"
        else
            echo "  Warning: create_helix_specs_branch function not available"
            BRANCH_EXISTS=false
        fi

        if [ "$BRANCH_EXISTS" = false ]; then
            echo "  Skipping worktree setup (branch doesn't exist)"
        else
            WORKTREE_PATH="$WORK_DIR/helix-specs"

            if [ ! -d "$WORKTREE_PATH" ]; then
                echo "  Creating design docs worktree at $WORKTREE_PATH..."
                if git -C "$PRIMARY_REPO_PATH" worktree add "$WORKTREE_PATH" helix-specs 2>&1; then
                    echo "  Design docs worktree ready at ~/work/helix-specs"
                    CURRENT_BRANCH=$(git -C "$WORKTREE_PATH" branch --show-current)
                    echo "  Current branch: $CURRENT_BRANCH"

                    # Pre-create task directory if specified (prevents "parent directory doesn't exist" errors)
                    if [ -n "$HELIX_SPEC_DIR_NAME" ]; then
                        TASK_DIR="$WORKTREE_PATH/design/tasks/$HELIX_SPEC_DIR_NAME"
                        mkdir -p "$TASK_DIR"
                        echo "  Created task directory: design/tasks/$HELIX_SPEC_DIR_NAME"
                    fi
                else
                    echo "  Warning: Failed to create worktree"
                fi
            else
                echo "  Design docs worktree already exists"
                CURRENT_BRANCH=$(git -C "$WORKTREE_PATH" branch --show-current 2>/dev/null || echo "unknown")
                echo "  Current branch: $CURRENT_BRANCH"

                # Pre-create task directory if specified (prevents "parent directory doesn't exist" errors)
                if [ -n "$HELIX_SPEC_DIR_NAME" ]; then
                    TASK_DIR="$WORKTREE_PATH/design/tasks/$HELIX_SPEC_DIR_NAME"
                    mkdir -p "$TASK_DIR"
                    echo "  Created task directory: design/tasks/$HELIX_SPEC_DIR_NAME"
                fi
            fi
        fi
    else
        echo "  Warning: Primary repository not found at $PRIMARY_REPO_PATH"
    fi
    echo ""
else
    echo "No primary repository specified"
    echo ""
fi

# =========================================
# Install Helix Git Hooks
# =========================================
if [ -f "/usr/local/bin/helix-git-hooks.sh" ]; then
    source "/usr/local/bin/helix-git-hooks.sh"
    install_helix_git_hooks
elif [ -f "$SCRIPT_DIR/helix-git-hooks.sh" ]; then
    source "$SCRIPT_DIR/helix-git-hooks.sh"
    install_helix_git_hooks
else
    echo "Warning: helix-git-hooks.sh not found - git hooks not installed"
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
    echo "  Claude: ~/.claude -> $CLAUDE_STATE_DIR"
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
    echo "  Created README.md to initialize workspace"
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
echo "  Zed keymap configured for system clipboard"

# Configure SSH agent if keys exist
if [ -d "$HOME/.ssh" ] && [ "$(ls -A $HOME/.ssh/*.key 2>/dev/null)" ]; then
    echo "  Setting up SSH agent for git access..."
    eval "$(ssh-agent -s)" > /dev/null
    for key in $HOME/.ssh/*.key; do
        ssh-add "$key" 2>/dev/null && echo "    Loaded SSH key: $(basename $key)"
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
        echo "  Primary: $HELIX_PRIMARY_REPO_NAME"
    fi
fi

# Add design docs worktree second (if exists)
DESIGN_DOCS_DIR="$WORK_DIR/helix-specs"
if [ -d "$DESIGN_DOCS_DIR" ]; then
    ZED_FOLDERS+=("$DESIGN_DOCS_DIR")
    echo "  Design docs: helix-specs"
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
            echo "  Other: $REPO_NAME"
        fi
    done
fi

# FAIL if no folders found - don't start Zed with empty workspace
if [ ${#ZED_FOLDERS[@]} -eq 0 ]; then
    echo ""
    echo "❌ ERROR: No repositories were cloned successfully"
    echo ""
    echo "Cannot start Zed without a proper workspace."
    echo "Check the errors above and fix the issue."
    echo ""
    echo "The terminal will stay open so you can debug."
    exit 1
fi

# Write folders to file for main script to read
printf '%s\n' "${ZED_FOLDERS[@]}" > "$FOLDERS_FILE"
echo ""
echo "Zed will open ${#ZED_FOLDERS[@]} folder(s)"
echo ""

# =========================================
# Run startup script (if exists)
# =========================================
# Note: Startup script failures are NOT fatal - we continue to Zed
# The user can see the error and fix it
STARTUP_SCRIPT="$WORK_DIR/helix-specs/.helix/startup.sh"
if [ -f "$STARTUP_SCRIPT" ]; then
    echo "========================================="
    echo "Running startup script..."
    echo "========================================="
    echo "Script: $STARTUP_SCRIPT"
    echo ""

    # Change to primary repo for running commands
    if [ -n "$HELIX_PRIMARY_REPO_NAME" ] && [ -d "$WORK_DIR/$HELIX_PRIMARY_REPO_NAME" ]; then
        cd "$WORK_DIR/$HELIX_PRIMARY_REPO_NAME"
        echo "Working directory: $HELIX_PRIMARY_REPO_NAME"
    fi
    echo ""

    # Run startup script but don't fail if it errors (user can debug in terminal)
    if bash -i "$STARTUP_SCRIPT"; then
        echo ""
        echo "✅ Startup script completed successfully"
    else
        EXIT_CODE=$?
        echo ""
        echo "❌ Startup script failed with exit code $EXIT_CODE"
        echo ""
        echo "You can debug this in the terminal after Zed starts."
    fi
    echo ""
fi

# =========================================
# Signal completion - Zed can now start
# =========================================
touch "$COMPLETE_SIGNAL"

echo "========================================="
echo "✅ Setup complete! Zed is starting..."
echo "========================================="
# The EXIT trap will show the interactive menu
