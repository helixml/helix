# Design: Rename Repo Directories in helix-specs Startup

## Overview

Modify the helix-specs startup script to rename numbered repo directories to their canonical names so the `./stack` script works correctly, while maintaining compatibility with container restarts.

## Root Cause

The API's `CreateRepository` function in `api/pkg/services/git_repository_service.go` (lines 135-143) auto-increments repository names when a user already has a repo with that name:

```go
// Auto-increment name if it already exists
baseName := request.Name
uniqueName := baseName
suffix := 1
for existingNames[uniqueName] {
    uniqueName = fmt.Sprintf("%s-%d", baseName, suffix)
    suffix++
}
```

So if you already have `helix`, `zed`, `qwen-code` repos, new ones become `helix-1`, `zed-1`, `qwen-code-1`.

## Why This Matters

The `./stack` script uses `$PROJECTS_ROOT` (parent of helix) and expects:
- `$PROJECTS_ROOT/zed`
- `$PROJECTS_ROOT/qwen-code`

When repos are cloned with numbered names, `./stack build-zed` and other commands fail.

## Critical: Container Restart Behavior

The `helix-workspace-setup.sh` script receives `HELIX_REPOSITORIES` and `HELIX_PRIMARY_REPO_NAME` from the API with the **DB repo names** (e.g., `zed-1`). These are used for:

1. **Clone skip check**: `if [ -d "$WORK_DIR/$REPO_NAME/.git" ]`
2. **Branch checkout**: `PRIMARY_REPO_PATH="$WORK_DIR/$HELIX_PRIMARY_REPO_NAME"`
3. **Worktree setup**: Uses `PRIMARY_REPO_PATH`
4. **Git hooks**: References `$WORK_DIR/$PRIMARY_REPO_NAME`
5. **Zed folders**: Adds paths based on repo names

If we simply rename `zed-1` → `zed`, on container restart the script won't find `~/work/zed-1/.git` and will re-clone, creating duplicate directories.

## Solution

Rename directories AND create symlinks for backward compatibility:

```bash
# Add to startup.sh before finding HELIX_DIR
cd ~/work

# Rename numbered repos to canonical names, with symlinks for API compatibility
for pattern in "helix-" "zed-" "qwen-code-"; do
    canonical="${pattern%-}"  # Remove trailing dash (e.g., "helix-" -> "helix")
    for numbered in ${pattern}[0-9]*; do
        [ -d "$numbered" ] || continue
        if [ ! -e "$canonical" ]; then
            mv "$numbered" "$canonical"
            ln -s "$canonical" "$numbered"  # Symlink so API still finds it
            echo "Renamed $numbered → $canonical (with symlink)"
        fi
    done
done
```

This ensures:
1. `./stack` finds `../zed`, `../qwen-code` (real directories)
2. `helix-workspace-setup.sh` finds `~/work/zed-1/.git` on restart (via symlink)
3. All paths work whether numbered or canonical

## Part 2: Resolve Symlinks in helix-workspace-setup.sh

To show canonical names in Zed's sidebar (e.g., `zed` instead of `zed-1`), modify `helix-3/desktop/shared/helix-workspace-setup.sh` to resolve symlinks when building the Zed folders list.

In the "Build list of folders for Zed" section (~line 483), change:

```bash
# Before: adds symlink path
ZED_FOLDERS+=("$PRIMARY_REPO_DIR")

# After: resolve symlink to canonical name
ZED_FOLDERS+=("$(readlink -f "$PRIMARY_REPO_DIR")")
```

This way if `$PRIMARY_REPO_DIR` is `~/work/zed-1` (a symlink to `~/work/zed`), Zed will open `~/work/zed` and show "zed" in the sidebar.

## Verification

Verified helix codebase - no code assumes numbered names are actual directories (not symlinks):
- Prompts use `/home/retro/work/helix-specs/`
- `sample_project_code_service.go` clones as `helix`, `zed`, `qwen-code`
- `helix-dev-setup.sh` uses canonical names
- `stack` script uses `$PROJECTS_ROOT/zed`, `$PROJECTS_ROOT/qwen-code`
