# Design: Fix Project Startup Script

## Architecture Overview

The Helix-in-Helix development setup involves:

- **helix-4** (`~/work/helix-4`): Main Helix codebase on `main` branch, contains `./stack` script
- **helix-specs** (`~/work/helix-specs`): Git worktree of helix-4 on `helix-specs` branch, contains design docs and `.helix/startup.sh`
- **zed-4, qwen-code-4**: Sister repositories needed by the `./stack` script

## Issue 1: Docker Compose Shim Bug

### Problem

The docker-shim at `desktop/docker-shim/compose.go` incorrectly passes "compose" as the first argument when calling the real compose plugin.

### Root Cause Analysis

When `docker compose version` is invoked:

1. Docker CLI calls the compose plugin at `/usr/libexec/docker/cli-plugins/docker-compose` (which is symlinked to docker-shim)
2. The shim receives `args = ["compose", "version"]`
3. The shim extracts `pluginName = "compose"` and processes the remaining args
4. It builds `finalArgs = ["compose", "-p", "helix-task-1124", "version"]`
5. It calls `execReal(ComposeRealPath, finalArgs)` where `ComposeRealPath = "/usr/libexec/docker/cli-plugins/docker-compose.real"`
6. `execReal` builds `argv = ["/usr/libexec/docker/cli-plugins/docker-compose.real", "compose", "-p", ...]`
7. The real plugin receives "compose" as its first argument and fails with "unknown docker command: compose compose"

### Verified Behavior

```bash
# Direct call to real plugin works:
/usr/libexec/docker/cli-plugins/docker-compose.real version
# Output: Docker Compose version v5.0.2

# With "compose" as first arg fails:
/usr/libexec/docker/cli-plugins/docker-compose.real compose version
# Output: unknown docker command: "compose compose"
```

### Solution

Remove the `pluginName` from `finalArgs` in `compose.go`. The Docker CLI plugin protocol does NOT expect the plugin to receive its own name as an argument - that's only passed by docker to the shim for routing purposes.

**Code change in `desktop/docker-shim/compose.go` around line 35-40:**

```go
// Build final arguments
finalArgs := make([]string, 0, len(newArgs)+len(projectArgs))
// REMOVED: if pluginName != "" { finalArgs = append(finalArgs, pluginName) }
finalArgs = append(finalArgs, projectArgs...)
finalArgs = append(finalArgs, newArgs...)
```

## Issue 2: helix-specs Not in Project Roots

### Problem

The helix-specs worktree is NOT being created, and therefore not included in the project roots. The `~/.helix-zed-folders` file only contains:
```
/home/retro/work/helix-4
/home/retro/work/qwen-code-4
/home/retro/work/zed-4
```

### Root Cause Analysis

The bug is in the branch checkout logic in `helix-workspace-setup.sh` (lines 250-345).

When a spec task uses the `helix-specs` branch:
- `HELIX_BRANCH_MODE=existing`
- `HELIX_WORKING_BRANCH=helix-specs`

The script does this (line 268-275):
```bash
if [ "$HELIX_BRANCH_MODE" = "existing" ]; then
    if [ -n "$HELIX_WORKING_BRANCH" ]; then
        git checkout "$HELIX_WORKING_BRANCH"  # Checks out helix-specs ON THE MAIN REPO!
```

This checks out the `helix-specs` branch **directly on the primary repo (helix-4)**, instead of:
1. Keeping the main repo on `main` branch
2. Creating a separate worktree for `helix-specs`

The consequences:
1. The main repo (helix-4) ends up on `helix-specs` branch, which has no code (only design docs)
2. The worktree creation code (lines 346-410) may fail or be skipped
3. The `./stack` script doesn't exist on helix-specs branch, so builds fail
4. `~/work/helix-specs` directory never gets created, so it's not added to ZED_FOLDERS

### Solution

The workspace setup script needs special handling for `helix-specs` branch:
1. When `HELIX_WORKING_BRANCH=helix-specs`, DON'T checkout that branch on the main repo
2. Keep the main repo on `main` branch (or `HELIX_BASE_BRANCH`)
3. Create the helix-specs worktree as usual
4. The worktree creation code already exists - just need to not corrupt the main repo first

**Code change in `helix-workspace-setup.sh` around line 268:**
```bash
if [ "$HELIX_BRANCH_MODE" = "existing" ]; then
    if [ -n "$HELIX_WORKING_BRANCH" ]; then
        # Special case: helix-specs branch uses a worktree, not direct checkout
        if [ "$HELIX_WORKING_BRANCH" = "helix-specs" ]; then
            echo "  Mode: helix-specs branch (will use worktree)"
            echo "  Keeping main repo on default branch"
            # Worktree will be created later in the script
        else
            echo "  Mode: Continue existing branch"
            echo "  Checking out branch: $HELIX_WORKING_BRANCH"
            # ... existing checkout logic
        fi
```

## Issue 3: Startup Script Assumptions

### Problem

The startup script at `helix-specs/.helix/startup.sh` has several issues:

1. **Directory renaming logic**: Renames `helix-4` to `helix` and creates symlinks. This works but:
   - The `./stack` script expects `../zed` and `../qwen-code` relative paths
   - After renaming, paths resolve correctly
   - However, the numbered directories (helix-4, zed-4, qwen-code-4) are what the API creates and what tools expect

2. **Branch assumption**: Script assumes helix repo is on main branch, but doesn't verify this

3. **Non-idempotent tmux**: `./stack start` may fail if tmux session already exists

### Solution

Update the startup script to:
1. Keep the symlink creation logic (it works correctly)
2. Add a check that the helix directory is on the `main` branch before building
3. Handle existing tmux sessions gracefully
4. Improve error messages

## Implementation Order

1. **Fix docker-shim** (main branch) - Unblocks all docker compose usage
2. **Add helix-specs to project roots** (main branch) - Allows tools to access design docs
3. **Improve startup script** (helix-specs branch) - Better error handling and idempotency

## Files to Modify

| File | Branch | Change |
|------|--------|--------|
| `desktop/docker-shim/compose.go` | main | Remove pluginName from finalArgs |
| `desktop/shared/helix-workspace-setup.sh` | main | Don't checkout helix-specs directly on main repo |
| `.helix/startup.sh` | helix-specs | Improve error handling, add branch check |