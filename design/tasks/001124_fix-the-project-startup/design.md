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
   - ✅ This works correctly - the worktree is created

3. **Zed/IDE Startup**:
   - ❌ The helix-specs worktree path is NOT included in the project roots
   - This means tools/AI can't access files in the helix-specs directory
   - Need to add `~/work/helix-specs` to the list of project root directories

4. **User's startup script** (`helix-specs/.helix/startup.sh`):
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

### 2. helix-specs Worktree Not in Project Roots

**Problem**: The helix-specs worktree is created correctly at `~/work/helix-specs`, but it's NOT included in the list of project root directories passed to Zed/the IDE.

**Root Cause**: When setting up the project roots for the IDE, only the cloned repositories are included. The helix-specs worktree is created as a separate step and isn't added to the project roots list.

**Where to Fix**: Find where project roots are configured for Zed/IDE startup and add the helix-specs worktree path.

**Investigation needed**:
1. Where does Hydra Executor or Zed launcher configure the project root directories?
2. Is it in the Zed workspace configuration?
3. Is it passed as command-line arguments or environment variables?
4. Should helix-specs be added automatically for any project with a primary repo?

**Solution Options**:
- **Option A**: Add helix-specs to project roots in Hydra Executor when launching Zed
- **Option B**: Update Zed workspace configuration after worktree is created
- **Option C**: Add helix-specs path to the list of repositories in DesktopAgent.RepositoryIDs

### 3. Startup Script Execution Context

**Problem**: The startup script needs to handle the actual directory structure (numbered directories like helix-4) and ensure the main repo is on the main branch.

**Current Behavior**: Script renames `helix-4 → helix` and creates symlinks. This works but could be cleaner.

**Solution**: Update the startup script to:
1. Work with the actual numbered directory names (helix-4, zed-4, qwen-code-4)
2. Ensure helix-4 is on main branch before running `./stack` commands
3. Verify the helix-specs worktree exists at ~/work/helix-specs
4. Provide clear error messages if worktree doesn't exist

### 4. Idempotency

**Current State**: Script partially handles re-runs (checks for existing symlinks, checks for tmux/yarn).

**Improvements Needed**:
- Check if yarn installation is in progress before retrying
- Handle case where build is already running
- Gracefully handle existing tmux sessions
- Add better error messages and continue/skip logic

## Design Decisions

1. **Add helix-specs to project roots automatically**: When a project has a primary repository, automatically include the helix-specs worktree as a project root
2. **Fix docker-shim in main branch**: This is a code bug that affects all compose usage (separate commit)
3. **Make startup script defensive**: Script should check if worktree exists and provide helpful error if not
4. **Find the right place to add helix-specs**: Need to identify where project roots are configured

## Things Learned from Codebase

- The `./stack` script has built-in Helix-in-Helix detection (`detect_helix_in_helix` function)
- Docker-shim provides path translation and BuildKit cache injection
- The project expects `$PROJECTS_ROOT/{zed,qwen-code}` to exist alongside helix
- Numbered directories (helix-4, etc.) are an API quirk from the git server cloning
- helix-workspace-setup.sh is responsible for ALL workspace prep (repos, branches, worktree, hooks)
- The startup script runs AFTER workspace-setup, so worktree should already exist
- `helix-specs-create.sh` creates the orphan branch if it doesn't exist remotely
- Worktree creation uses `git worktree add ~/work/helix-specs helix-specs`
- **The worktree IS created, it's just not visible to tools because it's not in project roots**

## Constraints

- Startup script must remain in helix-specs branch
- Docker-shim fix must go to main branch (different commit/PR)
- Project roots configuration must go to main branch (wherever that is)
- Script must work in Hydra desktop environment with DinD
- Must handle both privileged (host Docker) and non-privileged modes

## Open Questions

1. Where are project roots configured for Zed/IDE? (Hydra Executor? Zed config? Environment variables?)
2. Should helix-specs be added automatically for all projects with a primary repo?
3. Should it be added as a separate "repository" in RepositoryIDs or as a special case?
