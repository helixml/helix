#!/bin/bash
# Helix Git Hooks Helper
# Provides functions to install commit-msg hooks that automatically add
# Code-Ref and Spec-Ref trailers to commit messages for spec-code traceability.
#
# Usage: source this file, then call install_helix_git_hooks
#
# Environment variables used:
#   HELIX_PRIMARY_REPO_NAME - Name of primary code repository
#   HELIX_SPEC_DIR_NAME     - Name of spec task directory (e.g., "0042-add-dark-mode")
#   WORK_DIR                - Base work directory (default: ~/work)

WORK_DIR="${WORK_DIR:-$HOME/work}"

# install_helix_git_hooks - Install commit-msg hooks for helix-specs and code repos
# Should be called after repositories are cloned and worktree is set up
install_helix_git_hooks() {
    echo "========================================="
    echo "Installing Helix Git Hooks..."
    echo "========================================="

    local helix_specs_dir="$WORK_DIR/helix-specs"
    local primary_repo_name="${HELIX_PRIMARY_REPO_NAME:-}"
    local spec_dir_name="${HELIX_SPEC_DIR_NAME:-}"

    # Install hook for helix-specs (adds Code-Ref pointing to code repo)
    if [ -d "$helix_specs_dir/.git" ] || [ -f "$helix_specs_dir/.git" ]; then
        install_helix_specs_hook "$helix_specs_dir" "$primary_repo_name"
    else
        echo "  Skipping helix-specs hook (not a git repo)"
    fi

    # Install hook for primary code repo (adds Spec-Ref pointing to helix-specs)
    if [ -n "$primary_repo_name" ]; then
        local primary_repo_dir="$WORK_DIR/$primary_repo_name"
        if [ -d "$primary_repo_dir/.git" ]; then
            install_code_repo_hook "$primary_repo_dir" "$spec_dir_name"
        else
            echo "  Skipping $primary_repo_name hook (not a git repo)"
        fi
    fi

    # Install hooks for other code repos (same Spec-Ref hook)
    if [ -n "$HELIX_REPOSITORIES" ]; then
        IFS=',' read -ra REPOS <<< "$HELIX_REPOSITORIES"
        for REPO_SPEC in "${REPOS[@]}"; do
            IFS=':' read -r REPO_ID REPO_NAME REPO_TYPE <<< "$REPO_SPEC"

            # Skip internal repos and primary repo (already handled)
            if [ "$REPO_TYPE" = "internal" ] || [ "$REPO_NAME" = "$primary_repo_name" ]; then
                continue
            fi

            local repo_dir="$WORK_DIR/$REPO_NAME"
            if [ -d "$repo_dir/.git" ]; then
                install_code_repo_hook "$repo_dir" "$spec_dir_name"
            fi
        done
    fi

    echo ""
}

# install_helix_specs_hook - Install commit-msg hook for helix-specs worktree
# Adds: Code-Ref: <repo>/<branch>@<hash>
install_helix_specs_hook() {
    local specs_dir="$1"
    local primary_repo_name="$2"

    # For worktrees, .git is a file pointing to the main repo's .git
    local git_dir
    if [ -f "$specs_dir/.git" ]; then
        # It's a worktree - read the gitdir path
        git_dir=$(cat "$specs_dir/.git" | sed 's/gitdir: //')
    else
        git_dir="$specs_dir/.git"
    fi

    local hooks_dir="$git_dir/hooks"
    mkdir -p "$hooks_dir"

    cat > "$hooks_dir/commit-msg" << 'HOOK_EOF'
#!/bin/bash
# Helix commit-msg hook for helix-specs
# Adds Code-Ref trailer pointing to the code repository's current state

COMMIT_MSG_FILE="$1"
WORK_DIR="${WORK_DIR:-$HOME/work}"
PRIMARY_REPO_NAME="${HELIX_PRIMARY_REPO_NAME:-}"

# Skip if no primary repo configured
if [ -z "$PRIMARY_REPO_NAME" ]; then
    exit 0
fi

PRIMARY_REPO_DIR="$WORK_DIR/$PRIMARY_REPO_NAME"

# Skip if primary repo doesn't exist
if [ ! -d "$PRIMARY_REPO_DIR/.git" ]; then
    exit 0
fi

# Get current branch and commit from primary repo
CODE_BRANCH=$(git -C "$PRIMARY_REPO_DIR" branch --show-current 2>/dev/null || echo "unknown")
CODE_HASH=$(git -C "$PRIMARY_REPO_DIR" rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Check if Code-Ref already exists in commit message
if grep -q "^Code-Ref:" "$COMMIT_MSG_FILE"; then
    exit 0
fi

# Check if this is an empty commit message (or just comments)
if ! grep -v '^#' "$COMMIT_MSG_FILE" | grep -q '[^[:space:]]'; then
    exit 0
fi

# Append Code-Ref trailer
# Add blank line before trailer if message doesn't end with one
if [ -n "$(tail -c 1 "$COMMIT_MSG_FILE")" ]; then
    echo "" >> "$COMMIT_MSG_FILE"
fi
echo "" >> "$COMMIT_MSG_FILE"
echo "Code-Ref: $PRIMARY_REPO_NAME/$CODE_BRANCH@$CODE_HASH" >> "$COMMIT_MSG_FILE"
HOOK_EOF

    chmod +x "$hooks_dir/commit-msg"
    echo "  Installed helix-specs hook at $hooks_dir/commit-msg"
}

# install_code_repo_hook - Install commit-msg hook for code repository
# Adds: Spec-Ref: helix-specs@<hash>[:<spec-dir>]
install_code_repo_hook() {
    local repo_dir="$1"
    local spec_dir_name="$2"

    local hooks_dir="$repo_dir/.git/hooks"
    mkdir -p "$hooks_dir"

    cat > "$hooks_dir/commit-msg" << 'HOOK_EOF'
#!/bin/bash
# Helix commit-msg hook for code repositories
# Validates conventional commit format and adds Spec-Ref trailer

COMMIT_MSG_FILE="$1"
WORK_DIR="${WORK_DIR:-$HOME/work}"
SPEC_DIR_NAME="${HELIX_SPEC_DIR_NAME:-}"
HELIX_SPECS_DIR="$WORK_DIR/helix-specs"

# Check if this is an empty commit message (or just comments)
if ! grep -v '^#' "$COMMIT_MSG_FILE" | grep -q '[^[:space:]]'; then
    exit 0
fi

# Get the first non-comment, non-blank line (the subject)
SUBJECT=$(grep -v '^#' "$COMMIT_MSG_FILE" | grep -m1 '[^[:space:]]')

# Skip validation for merge/revert commits (git generates these)
case "$SUBJECT" in
    "Merge "*|"Revert "*|"fixup! "*|"squash! "*|"amend! "*)
        ;;
    *)
        # Validate conventional commit format
        CONVENTIONAL_RE='^(feat|fix|refactor|chore|docs|test|style|perf|ci|build|revert)(\([a-z0-9._/-]+\))?!?: .+'
        if ! echo "$SUBJECT" | grep -qE "$CONVENTIONAL_RE"; then
            echo "ERROR: Commit message must use conventional commit format." >&2
            echo "" >&2
            echo "  Format: type(scope): description" >&2
            echo "  Types:  feat, fix, refactor, chore, docs, test, style, perf, ci, build, revert" >&2
            echo "  Scope:  optional (e.g., api, frontend, specs)" >&2
            echo "" >&2
            echo "  Examples:" >&2
            echo "    feat(api): add PR content reading from helix-specs" >&2
            echo "    fix(frontend): handle empty task list" >&2
            echo "    chore: bump dependency versions" >&2
            echo "" >&2
            echo "  Got: $SUBJECT" >&2
            exit 1
        fi
        # Subject length check (warn, don't fail)
        if [ ${#SUBJECT} -gt 72 ]; then
            echo "WARNING: Commit subject is ${#SUBJECT} chars (recommend ≤ 72)." >&2
        fi
        ;;
esac

# Skip Spec-Ref if helix-specs doesn't exist
if [ ! -d "$HELIX_SPECS_DIR" ]; then
    exit 0
fi

# Get current commit from helix-specs
SPECS_HASH=$(git -C "$HELIX_SPECS_DIR" rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Check if Spec-Ref already exists in commit message
if grep -q "^Spec-Ref:" "$COMMIT_MSG_FILE"; then
    exit 0
fi

# Build Spec-Ref with optional spec directory
SPEC_REF="helix-specs@$SPECS_HASH"
if [ -n "$SPEC_DIR_NAME" ]; then
    SPEC_REF="helix-specs@$SPECS_HASH:$SPEC_DIR_NAME"
fi

# Append Spec-Ref trailer
# Add blank line before trailer if message doesn't end with one
if [ -n "$(tail -c 1 "$COMMIT_MSG_FILE")" ]; then
    echo "" >> "$COMMIT_MSG_FILE"
fi
echo "" >> "$COMMIT_MSG_FILE"
echo "Spec-Ref: $SPEC_REF" >> "$COMMIT_MSG_FILE"
HOOK_EOF

    chmod +x "$hooks_dir/commit-msg"
    echo "  Installed code repo hook at $hooks_dir/commit-msg"
}
