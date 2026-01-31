#!/bin/bash
#
# helix-specs-create.sh - Git helper functions for Helix workspace setup
#
# This script is sourced by start-zed-helix.sh and tested by test-helix-specs-creation.sh.
# The logic handles various edge cases:
# - Empty repositories (no commits)
# - Detached HEAD state
# - Dirty working directory (uncommitted changes)
# - Non-standard default branches (main vs master)
# - Azure DevOps repos that don't set refs/remotes/origin/HEAD
#
# Usage:
#   source helix-specs-create.sh
#   DEFAULT_BRANCH=$(get_default_branch "/path/to/repo")
#   create_helix_specs_branch "/path/to/repo"
#
# Returns:
#   get_default_branch: Outputs the default branch name to stdout
#   create_helix_specs_branch: Sets HELIX_SPECS_BRANCH_EXISTS=true/false
#
# Testing:
#   Run tests with: ./test-helix-specs-creation.sh
#   Tests verify all edge cases work correctly.

# Get the default branch for a repository
# Handles Azure DevOps repos that don't set refs/remotes/origin/HEAD
# Arguments:
#   $1 - Path to the git repository
# Output:
#   Prints the default branch name to stdout
get_default_branch() {
    local REPO_PATH="$1"
    local BRANCH=""

    if [ -z "$REPO_PATH" ] || [ ! -d "$REPO_PATH/.git" ]; then
        echo "main"
        return
    fi

    # Try to get default branch from symbolic ref (GitHub sets this)
    BRANCH=$(git -C "$REPO_PATH" symbolic-ref refs/remotes/origin/HEAD 2>/dev/null | sed 's@^refs/remotes/origin/@@')

    # If that fails (Azure DevOps often doesn't set this), check for common branch names
    if [ -z "$BRANCH" ]; then
        if git -C "$REPO_PATH" show-ref --verify refs/remotes/origin/main >/dev/null 2>&1; then
            BRANCH="main"
        elif git -C "$REPO_PATH" show-ref --verify refs/remotes/origin/master >/dev/null 2>&1; then
            BRANCH="master"
        else
            # Check current branch as fallback (for empty repos)
            BRANCH=$(git -C "$REPO_PATH" branch --show-current 2>/dev/null)
            if [ -z "$BRANCH" ]; then
                BRANCH="main"  # Last resort
            fi
        fi
    fi

    echo "$BRANCH"
}

# Create helix-specs orphan branch if it doesn't exist
# Arguments:
#   $1 - Path to the git repository
create_helix_specs_branch() {
    local REPO_PATH="$1"

    if [ -z "$REPO_PATH" ]; then
        echo "Error: Repository path required"
        HELIX_SPECS_BRANCH_EXISTS=false
        return 1
    fi

    # Check if repo exists
    if [ ! -d "$REPO_PATH/.git" ]; then
        echo "Error: Not a git repository: $REPO_PATH"
        HELIX_SPECS_BRANCH_EXISTS=false
        return 1
    fi

    # Check if helix-specs branch already exists
    HELIX_SPECS_BRANCH_EXISTS=false

    if git -C "$REPO_PATH" show-ref --verify refs/remotes/origin/helix-specs >/dev/null 2>&1; then
        echo "helix-specs branch exists on remote"
        HELIX_SPECS_BRANCH_EXISTS=true
        return 0
    elif git -C "$REPO_PATH" rev-parse --verify helix-specs >/dev/null 2>&1; then
        echo "helix-specs branch exists locally"
        HELIX_SPECS_BRANCH_EXISTS=true
        return 0
    fi

    # Branch doesn't exist - need to create it
    echo "Creating helix-specs orphan branch..."

    # Detect the current branch (what we need to return to after creating orphan)
    # For empty repos, this is whatever branch git init created (usually master)
    local CURRENT_BRANCH=$(git -C "$REPO_PATH" branch --show-current 2>/dev/null)

    # Handle detached HEAD - save the commit hash to return to
    local DETACHED_HEAD=""
    if [ -z "$CURRENT_BRANCH" ]; then
        DETACHED_HEAD=$(git -C "$REPO_PATH" rev-parse HEAD 2>/dev/null || echo "")
        if [ -n "$DETACHED_HEAD" ]; then
            echo "  HEAD is detached at $DETACHED_HEAD"
        fi
    fi

    # Stash any uncommitted changes (prevents checkout failures)
    local STASHED=false
    if git -C "$REPO_PATH" diff --quiet 2>/dev/null && \
       git -C "$REPO_PATH" diff --cached --quiet 2>/dev/null; then
        : # Working directory is clean
    else
        echo "  Stashing uncommitted changes..."
        if git -C "$REPO_PATH" stash push -m "helix-specs-setup" 2>&1; then
            STASHED=true
        fi
    fi

    # Detect the default branch from remote (could be main or master)
    # Uses shared get_default_branch function that handles Azure DevOps repos
    local REPO_DEFAULT_BRANCH=$(get_default_branch "$REPO_PATH")

    echo "  Current branch: ${CURRENT_BRANCH:-detached}, will return to: $REPO_DEFAULT_BRANCH"

    # Check if repo is empty (no commits on any branch)
    local REPO_IS_EMPTY=false
    if ! git -C "$REPO_PATH" rev-parse HEAD >/dev/null 2>&1; then
        REPO_IS_EMPTY=true
        echo "  Repository is empty (no commits yet)"
    fi

    # 1. Create orphan branch (no parent, no files)
    # 2. Remove any staged files (only if not empty repo)
    # 3. Commit empty state
    # 4. Push to remote
    # 5. Switch back to original branch
    local CREATE_SUCCESS=false
    if git -C "$REPO_PATH" checkout --orphan helix-specs 2>&1; then
        # Only try to remove files if repo has content
        # Suppress errors (pathspec '.' fails on empty working dir)
        if [ "$REPO_IS_EMPTY" = false ]; then
            git -C "$REPO_PATH" rm -rf . >/dev/null 2>&1 || true
        fi
        # Reset index for clean state
        git -C "$REPO_PATH" reset >/dev/null 2>&1 || true

        if git -C "$REPO_PATH" commit --allow-empty -m "Initialize helix-specs branch" 2>&1 && \
           git -C "$REPO_PATH" push origin helix-specs 2>&1; then
            echo "  helix-specs orphan branch created and pushed"
            CREATE_SUCCESS=true
            HELIX_SPECS_BRANCH_EXISTS=true
        else
            echo "  Failed to push helix-specs (may not have push permission)"
        fi
    fi

    # Return to original state
    # Determine which branch to return to (prefer CURRENT_BRANCH, fallback to REPO_DEFAULT_BRANCH)
    local RETURN_BRANCH="${CURRENT_BRANCH:-$REPO_DEFAULT_BRANCH}"

    if [ "$CREATE_SUCCESS" = true ]; then
        if [ "$REPO_IS_EMPTY" = true ]; then
            # For empty repos, create the default branch with an initial commit
            # so we have somewhere to return to
            if ! git -C "$REPO_PATH" show-ref --verify "refs/heads/$RETURN_BRANCH" >/dev/null 2>&1; then
                echo "  Creating initial $RETURN_BRANCH branch..."
                git -C "$REPO_PATH" checkout --orphan "$RETURN_BRANCH" 2>&1 || true
                git -C "$REPO_PATH" commit --allow-empty -m "Initial commit" 2>&1 || true
                git -C "$REPO_PATH" push -u origin "$RETURN_BRANCH" 2>&1 || true
            else
                git -C "$REPO_PATH" checkout "$RETURN_BRANCH" >/dev/null 2>&1 || true
            fi
        elif [ -n "$DETACHED_HEAD" ]; then
            # Return to detached HEAD state
            echo "  Returning to detached HEAD at $DETACHED_HEAD..."
            git -C "$REPO_PATH" checkout "$DETACHED_HEAD" >/dev/null 2>&1 || true
        else
            # Return to original branch (try current branch first, then default)
            if ! git -C "$REPO_PATH" checkout "$RETURN_BRANCH" >/dev/null 2>&1; then
                # If that fails, try the other common default branch names
                git -C "$REPO_PATH" checkout main >/dev/null 2>&1 || \
                git -C "$REPO_PATH" checkout master >/dev/null 2>&1 || true
            fi
        fi
        echo "  Returned to $RETURN_BRANCH"
    else
        echo "  Failed to create helix-specs orphan branch"
        # Try to return to original state
        if [ -n "$DETACHED_HEAD" ]; then
            git -C "$REPO_PATH" checkout "$DETACHED_HEAD" >/dev/null 2>&1 || true
        elif [ -n "$RETURN_BRANCH" ]; then
            git -C "$REPO_PATH" checkout "$RETURN_BRANCH" >/dev/null 2>&1 || \
            git -C "$REPO_PATH" checkout main >/dev/null 2>&1 || \
            git -C "$REPO_PATH" checkout master >/dev/null 2>&1 || true
        fi
    fi

    # Restore stashed changes
    if [ "$STASHED" = true ]; then
        echo "  Restoring stashed changes..."
        git -C "$REPO_PATH" stash pop 2>&1 || true
    fi

    return 0
}
