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

## Known Limitation

Zed's sidebar will still show the numbered names (e.g., `zed-1`) because `helix-workspace-setup.sh` adds folders using `$HELIX_PRIMARY_REPO_NAME` from the API. Fixing this would require modifying the built-in script to resolve symlinks to their canonical names - a larger change that can be done separately if desired.

The symlinks solve the critical functional issue (no duplicate clones on restart).

## Verification

Verified helix codebase - no code assumes numbered names are actual directories (not symlinks):
- Prompts use `/home/retro/work/helix-specs/`
- `sample_project_code_service.go` clones as `helix`, `zed`, `qwen-code`
- `helix-dev-setup.sh` uses canonical names
- `stack` script uses `$PROJECTS_ROOT/zed`, `$PROJECTS_ROOT/qwen-code`
