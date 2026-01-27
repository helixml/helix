# Requirements: Rename Repo Directories in helix-specs Startup

## User Story

As a developer working on helix-in-helix development, I want the repo directories to be named consistently (`helix`, `zed`, `qwen-code`) so that the `./stack` script works correctly since it expects `../zed`, `../qwen-code` etc. to exist.

## Background

When a user already has repos with names like `helix`, `zed`, `qwen-code`, the API's `CreateRepository` function auto-increments to avoid conflicts: `helix-1`, `zed-1`, `qwen-code-1`, then `helix-2`, etc.

The `./stack` script in helix uses `$PROJECTS_ROOT/zed` and `$PROJECTS_ROOT/qwen-code` paths, requiring consistent naming.

**Root cause**: `api/pkg/services/git_repository_service.go` function `CreateRepository` (lines 135-143) auto-increments repo names to avoid duplicates.

## Critical Issue: Container Restart Behavior

The workspace setup script (`helix-workspace-setup.sh`) uses `HELIX_REPOSITORIES` env var which contains repo names from the DB (e.g., `zed-1`). On restart:

1. Script checks `if [ -d "$WORK_DIR/$REPO_NAME/.git" ]` (e.g., `~/work/zed-1/.git`)
2. If we renamed `zed-1` → `zed`, the check fails
3. Script re-clones into `~/work/zed-1` - now we have BOTH directories!

The same `HELIX_PRIMARY_REPO_NAME` is used for:
- Branch checkout
- Worktree setup  
- Git hooks installation
- Zed folder paths

## Acceptance Criteria

1. **Startup script in helix-specs renames repos correctly**
   - `helix-1`, `helix-2`, etc. → `helix`
   - `zed-1`, `zed-2`, etc. → `zed`
   - `qwen-code-1`, `qwen-code-2`, etc. → `qwen-code`

2. **Create symlinks for API compatibility**
   - After renaming `zed-1` → `zed`, create symlink `zed-1` → `zed`
   - This ensures `helix-workspace-setup.sh` finds the repo on restart

3. **Idempotent operation**
   - Script should handle already-renamed repos gracefully
   - Script should handle repos that were never numbered

## Out of Scope

- Changing the API's auto-increment logic (that's a separate concern for avoiding DB conflicts)