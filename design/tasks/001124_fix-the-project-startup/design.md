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

The helix-specs worktree is created correctly at `~/work/helix-specs` by `helix-workspace-setup.sh`, but it's NOT included in the list of project root directories passed to Zed. This means AI tools and the IDE cannot access files in the helix-specs directory.

### Where to Fix

The project roots are configured when launching Zed. Need to investigate:
- `api/pkg/external-agent/hydra_executor.go` - builds the DesktopAgent with RepositoryIDs
- How Zed workspace is configured with project directories
- Whether helix-specs should be added as a pseudo-repository or handled specially

### Solution

Add `~/work/helix-specs` to the project roots after the worktree is created. This could be done by:
1. Adding it to the Zed workspace configuration
2. Treating it as an additional repository path in the executor
3. Modifying the workspace setup to register it with Zed

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
| TBD (project roots config) | main | Add helix-specs worktree to project roots |
| `.helix/startup.sh` | helix-specs | Improve error handling, add branch check |