# Design: Fix Project Startup Script

## Architecture Overview

The Helix project uses a dual-branch setup:
- **main branch** (`/home/retro/work/helix`): Contains the actual Helix codebase with `./stack` script
- **helix-specs branch** (`/home/retro/work/helix-specs`): Git worktree containing design docs and `.helix/startup.sh`

## Key Issues and Solutions

### 1. Docker Compose Shim Bug

**Problem**: The docker-shim at `desktop/docker-shim/compose.go` incorrectly adds "compose" as the first argument when calling the real compose plugin, causing double-compose error.

**Root Cause**: Docker CLI plugins don't expect "compose" as the first argument when called directly. The shim preserves the "compose" argument from `docker compose` invocations and passes it to `docker-compose.real`, which already knows it's compose.

**Solution**: Remove the pluginName from finalArgs when calling the real compose plugin. The plugin protocol expects arguments AFTER the plugin name, not including it.

**Code Change Required**:
```go
// In compose.go runCompose() function:
// Don't add pluginName to finalArgs when calling the real plugin
finalArgs := make([]string, 0, len(newArgs)+len(projectArgs))
// Remove: if pluginName != "" { finalArgs = append(finalArgs, pluginName) }
finalArgs = append(finalArgs, projectArgs...)
finalArgs = append(finalArgs, newArgs...)
```

### 2. Startup Script Execution Context

**Problem**: The script needs to run from the helix-specs worktree but access files from the main branch.

**Solution**: The script should:
1. Detect its own location (should be in helix-specs worktree)
2. Find the main Helix repo (parent directory's `helix` subdirectory)
3. Ensure main repo is on the main branch before running build commands

**Pattern Discovered**: The script already handles numbered directory renaming (helix-4 → helix). This is correct and should be preserved.

### 3. Idempotency

**Current State**: Script partially handles re-runs (checks for existing symlinks, checks for tmux/yarn).

**Improvements Needed**:
- Check if yarn installation is in progress before retrying
- Handle case where build is already running
- Gracefully handle existing tmux sessions
- Add better error messages and continue/skip logic

## Design Decisions

1. **Keep the worktree setup**: helix-specs as a separate worktree allows editing startup script while main branch remains untouched
2. **Fix docker-shim in main branch**: This is a code bug that affects all compose usage
3. **Make startup script branch-aware**: Script should check and ensure correct branch before operations
4. **Add safety checks**: Verify prerequisites before attempting builds

## Things Learned from Codebase

- The `./stack` script has built-in Helix-in-Helix detection (`detect_helix_in_helix` function)
- Docker-shim provides path translation and BuildKit cache injection
- The project expects `$PROJECTS_ROOT/{zed,qwen-code}` to exist alongside helix
- Numbered directories (helix-4, etc.) are an API quirk that must be handled

## Constraints

- Startup script must remain in helix-specs branch
- Docker-shim fix must go to main branch (different commit/PR)
- Script must work in Hydra desktop environment with DinD
- Must handle both privileged (host Docker) and non-privileged modes
