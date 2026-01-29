# Design: Fix Project Startup Script

## Architecture Overview

The Helix-in-Helix development setup involves:

- **helix-4** (`~/work/helix-4`): Main Helix codebase, should be on a feature branch for code changes
- **helix-specs** (`~/work/helix-specs`): Git worktree on `helix-specs` branch, contains design docs and `.helix/startup.sh`
- **zed-4, qwen-code-4**: Sister repositories needed by the `./stack` script

## Expected Session Setup

For a task like "fix the startup script", the correct setup should be:

1. `~/work/helix-4` on `feature/001124-fix-the-project-startup` branch (for code changes like docker-shim fix)
2. `~/work/helix-specs` worktree on `helix-specs` branch (for design docs and startup script changes)

Both should exist simultaneously. The helix-specs worktree should ALWAYS be created, regardless of what feature branch the main repo is on.

## Issue 1: Docker Compose Shim Bug

### Problem

The docker-shim at `desktop/docker-shim/compose.go` incorrectly passes "compose" as the first argument when calling the real compose plugin.

### Root Cause Analysis

When `docker compose version` is invoked:

1. Docker CLI calls the compose plugin at `/usr/libexec/docker/cli-plugins/docker-compose` (symlinked to docker-shim)
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

Remove the `pluginName` from `finalArgs` in `compose.go`. The Docker CLI plugin protocol does NOT expect the plugin to receive its own name as an argument.

**Code change in `desktop/docker-shim/compose.go` around line 35-40:**

```go
// Build final arguments
finalArgs := make([]string, 0, len(newArgs)+len(projectArgs))
// REMOVED: if pluginName != "" { finalArgs = append(finalArgs, pluginName) }
finalArgs = append(finalArgs, projectArgs...)
finalArgs = append(finalArgs, newArgs...)
```

## Issue 2: Wrong Branch Configuration

### Problem

The current session has:
```
HELIX_WORKING_BRANCH=helix-specs
HELIX_BASE_BRANCH=helix-specs
HELIX_BRANCH_MODE=existing
```

This is incorrect. The task should have a feature branch (e.g., `feature/001124-fix-the-project-startup`) as the working branch, NOT `helix-specs`.

### Consequences

When `HELIX_WORKING_BRANCH=helix-specs`:
1. The workspace setup does `git checkout helix-specs` on helix-4
2. helix-4 ends up on helix-specs branch (no code, no `./stack`)
3. Worktree creation fails (can't create worktree for currently checked out branch)
4. `~/work/helix-specs` is never created
5. ZED_FOLDERS doesn't include helix-specs

### Root Cause

The task was created with the wrong branch configuration. Tasks should always get a feature branch name, even when they involve editing files in helix-specs.

### Solution

Two fixes needed:

1. **Task creation fix**: Ensure tasks always get a proper feature branch name, not `helix-specs`

2. **Workspace setup defensive fix**: Even if `HELIX_WORKING_BRANCH=helix-specs` is passed (incorrectly), the workspace setup should handle it gracefully by:
   - NOT checking out helix-specs directly on the main repo
   - Creating the helix-specs worktree instead

**Code change in `helix-workspace-setup.sh` around line 268:**
```bash
if [ "$HELIX_BRANCH_MODE" = "existing" ]; then
    if [ -n "$HELIX_WORKING_BRANCH" ]; then
        # Special case: helix-specs should use worktree, not direct checkout
        if [ "$HELIX_WORKING_BRANCH" = "helix-specs" ]; then
            echo "  Warning: helix-specs should not be the working branch"
            echo "  Keeping main repo on default branch, will create worktree instead"
        else
            echo "  Mode: Continue existing branch"
            echo "  Checking out branch: $HELIX_WORKING_BRANCH"
            # ... existing checkout logic
        fi
```

## Issue 3: Startup Script Assumptions

### Problem

The startup script at `helix-specs/.helix/startup.sh` has minor issues:

1. Doesn't verify the main repo is on the correct branch before building
2. Doesn't handle existing tmux sessions gracefully

### Solution

Update the startup script to:
1. Check that helix directory has the `./stack` script before trying to run it
2. Handle existing tmux sessions
3. Better error messages

## Implementation Order

1. **Fix docker-shim** (main branch) - Unblocks all docker compose usage
2. **Fix task creation** (main branch) - Ensure tasks get feature branches, not helix-specs
3. **Fix workspace setup** (main branch) - Defensive handling of helix-specs as working branch
4. **Improve startup script** (helix-specs branch) - Better error handling

## Files to Modify

| File | Branch | Change |
|------|--------|--------|
| `desktop/docker-shim/compose.go` | main | Remove pluginName from finalArgs |
| Task creation code (TBD) | main | Ensure tasks get feature branch names |
| `desktop/shared/helix-workspace-setup.sh` | main | Defensive handling of helix-specs branch |
| `.helix/startup.sh` | helix-specs | Improve error handling |