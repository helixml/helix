# Design: Fix Project Startup Script

## Architecture Overview

The Helix project uses a dual-branch setup:
- **main branch** (`~/work/helix-4`): Contains the actual Helix codebase with `./stack` script
- **helix-specs branch** (`~/work/helix-specs`): Git worktree containing design docs and `.helix/startup.sh`

## Workspace Setup Flow

1. **Hydra Executor** (`api/pkg/external-agent/hydra_executor.go`):
   - Sets `HELIX_REPOSITORIES` env var with repo IDs, names, and types
   - Sets `HELIX_PRIMARY_REPO_NAME` for the main repository
   - Starts dev container with these environment variables

2. **helix-workspace-setup.sh** (`desktop/shared/helix-workspace-setup.sh`):
   - Sources `helix-specs-create.sh` for helper functions
   - Clones all repositories listed in `HELIX_REPOSITORIES`
   - Calls `create_helix_specs_branch()` to create helix-specs orphan branch if needed
   - Creates helix-specs worktree at `~/work/helix-specs` using `git worktree add`
   - Should create the worktree automatically, but investigation needed to find why it's not working

3. **User's startup script** (`helix-specs/.helix/startup.sh`):
   - Runs AFTER workspace setup
   - Should find repos already cloned and helix-specs worktree already created
   - Performs project-specific build and startup tasks

## Key Issues and Solutions

### 1. Docker Compose Shim Bug

**Problem**: The docker-shim at `desktop/docker-shim/compose.go` incorrectly adds "compose" as the first argument when calling the real compose plugin, causing double-compose error.

**Root Cause**: Docker CLI plugins don't expect "compose" as the first argument when called directly. The shim preserves the "compose" argument from `docker compose` invocations and passes it to `docker-compose.real`, which already knows it's compose.

**Solution**: Remove the pluginName from finalArgs when calling the real compose plugin.

**Code Change Required**:
```go
// In compose.go runCompose() function (around line 40):
// Don't add pluginName to finalArgs when calling the real plugin
finalArgs := make([]string, 0, len(newArgs)+len(projectArgs))
// REMOVE THIS LINE: if pluginName != "" { finalArgs = append(finalArgs, pluginName) }
finalArgs = append(finalArgs, projectArgs...)
finalArgs = append(finalArgs, newArgs...)
```

### 2. Missing helix-specs Worktree

**Problem**: The helix-specs worktree is NOT being created automatically during project setup, even though the code to create it exists in `helix-workspace-setup.sh`.

**Root Cause** (to investigate): 
- The worktree creation logic exists at lines 346-400 of helix-workspace-setup.sh
- It requires `HELIX_PRIMARY_REPO_NAME` to be set
- It requires `create_helix_specs_branch` function from `helix-specs-create.sh`
- Need to investigate: Is the function being sourced? Is the primary repo name set? Is the branch creation failing?

**Solution**: Debug and fix the worktree creation in helix-workspace-setup.sh

**Investigation needed**:
1. Check if `HELIX_PRIMARY_REPO_NAME` is being set correctly by Hydra Executor
2. Check if `helix-specs-create.sh` is being sourced successfully  
3. Check if `create_helix_specs_branch()` is running and succeeding
4. Add better logging to identify where the worktree creation is failing

### 3. Startup Script Execution Context

**Problem**: The startup script needs to handle the actual directory structure (numbered directories like helix-4) and ensure the main repo is on the main branch.

**Current Behavior**: Script renames `helix-4 → helix` and creates symlinks. This works but could be cleaner.

**Solution**: Update the startup script to:
1. Work with the actual numbered directory names (helix-4, zed-4, qwen-code-4)
2. Ensure helix-4 is on main branch before running `./stack` commands
3. Find the helix-specs worktree (should be at ~/work/helix-specs after workspace setup)
4. Provide clear error messages if worktree doesn't exist

### 4. Idempotency

**Current State**: Script partially handles re-runs (checks for existing symlinks, checks for tmux/yarn).

**Improvements Needed**:
- Check if yarn installation is in progress before retrying
- Handle case where build is already running
- Gracefully handle existing tmux sessions
- Add better error messages and continue/skip logic

## Design Decisions

1. **Keep the worktree setup in helix-workspace-setup.sh**: This is the right place - it runs before the user's startup script
2. **Fix docker-shim in main branch**: This is a code bug that affects all compose usage (separate commit)
3. **Make startup script defensive**: Script should check if worktree exists and provide helpful error if not
4. **Add debugging**: Add better logging to workspace-setup.sh to see why worktree isn't being created

## Things Learned from Codebase

- The `./stack` script has built-in Helix-in-Helix detection (`detect_helix_in_helix` function)
- Docker-shim provides path translation and BuildKit cache injection
- The project expects `$PROJECTS_ROOT/{zed,qwen-code}` to exist alongside helix
- Numbered directories (helix-4, etc.) are an API quirk from the git server cloning
- helix-workspace-setup.sh is responsible for ALL workspace prep (repos, branches, worktree, hooks)
- The startup script runs AFTER workspace-setup, so worktree should already exist
- `helix-specs-create.sh` creates the orphan branch if it doesn't exist remotely
- Worktree creation uses `git worktree add ~/work/helix-specs helix-specs`

## Constraints

- Startup script must remain in helix-specs branch  
- Docker-shim fix must go to main branch (different commit/PR)
- Worktree setup fix must go to main branch (helix-workspace-setup.sh)
- Script must work in Hydra desktop environment with DinD
- Must handle both privileged (host Docker) and non-privileged modes
