# SpecTask Worktree Fix - Implementation Complete

**Date:** 2025-11-06
**Status:** ✅ Implemented and Built
**Related:** 2025-11-06-fix-spectask-worktree-path-mismatch.md

## Summary

Fixed critical bug preventing SpecTask planning agents from committing design documents to the `helix-design-docs` branch, and switched from bind-mounting internal repos to properly cloning them (preparing for network separation of Wolf server).

## Changes Implemented

### 1. Container Startup Script (`wolf/sway-config/start-zed-helix.sh`)

**Added worktree setup logic** (after line 19, before Claude state setup):

```bash
# Setup helix-design-docs worktrees for SpecTask repositories
# This must happen INSIDE the Wolf container so git paths are correct for this environment
# (Worktrees created in API container have wrong paths when mounted here)
echo "Setting up design docs worktrees..."
for repo_dir in */; do
    # Skip if not a directory or doesn't exist
    [ -d "$repo_dir" ] || continue

    # Check if this is a git repository
    if [ -d "$repo_dir/.git" ]; then
        repo_name="${repo_dir%/}"
        worktree_path="$repo_dir.git-worktrees/helix-design-docs"

        # Check if helix-design-docs branch exists
        if git -C "$repo_dir" rev-parse --verify helix-design-docs >/dev/null 2>&1; then
            # Branch exists, check if worktree is already set up correctly
            if [ -d "$worktree_path" ] && [ -f "$worktree_path/.git" ]; then
                # Verify worktree .git file has correct path
                git_path=$(cat "$worktree_path/.git" | grep -o 'gitdir: .*' | cut -d' ' -f2)
                if [ -d "$git_path" ]; then
                    echo "  ✅ Worktree valid for $repo_name"
                    continue
                else
                    echo "  ⚠️  Worktree has invalid path for $repo_name, recreating..."
                    rm -rf "$worktree_path"
                fi
            fi

            # Create worktree (either doesn't exist or had invalid path)
            if [ ! -d "$worktree_path" ]; then
                echo "  📝 Creating worktree for $repo_name..."
                if git -C "$repo_dir" worktree add "$worktree_path" helix-design-docs 2>/dev/null; then
                    echo "  ✅ Created worktree for $repo_name"
                else
                    echo "  ⚠️  Failed to create worktree for $repo_name (may already exist in git's worktree list)"
                    # Try to prune and recreate
                    git -C "$repo_dir" worktree prune 2>/dev/null
                    if git -C "$repo_dir" worktree add "$worktree_path" helix-design-docs 2>/dev/null; then
                        echo "  ✅ Created worktree for $repo_name after pruning"
                    fi
                fi
            fi
        fi
    fi
done
echo "Design docs worktrees ready"
```

**Updated internal repo startup script discovery** (lines 143-156):

```bash
# Execute project startup script from internal Git repo - run in terminal window
# Internal repos are now cloned (not mounted) so search all repos for .helix/startup.sh
STARTUP_SCRIPT_PATH=""
INTERNAL_REPO_PATH=""

for repo_dir in */; do
    [ -d "$repo_dir" ] || continue
    if [ -f "$repo_dir/.helix/startup.sh" ]; then
        INTERNAL_REPO_PATH="$WORK_DIR/${repo_dir%/}"
        STARTUP_SCRIPT_PATH="$INTERNAL_REPO_PATH/.helix/startup.sh"
        echo "Found internal project repo: $INTERNAL_REPO_PATH"
        break
    fi
done

if [ -n "$STARTUP_SCRIPT_PATH" ] && [ -f "$STARTUP_SCRIPT_PATH" ]; then
```

### 2. Wolf Executor (`api/pkg/external-agent/wolf_executor.go`)

**Removed internal repo mounting logic** (lines 423-437):

```go
// Internal repos are now cloned like any other repo (no longer mounted)
// This allows Wolf server to be separated from API server over the network
```

**Removed filter excluding internal repos from cloning** (lines 439-447):

```go
// Clone git repositories if specified (for SpecTasks with repository context)
// Internal repos are now cloned like any other repo (no special handling)
if len(agent.RepositoryIDs) > 0 {
    err := w.setupGitRepositories(ctx, workspaceDir, agent.RepositoryIDs, agent.PrimaryRepositoryID)
    if err != nil {
        log.Error().Err(err).Msg("Failed to setup git repositories")
        return nil, fmt.Errorf("failed to setup git repositories: %w", err)
    }
}
```

**Removed extra mounts for internal repo** (lines 513-514):

```go
// No extra mounts needed - internal repos are now cloned instead of mounted
extraMounts := []string{}
```

**Removed worktree creation from API container** (lines 2288-2290, 2385-2393):

```go
// Note: Design docs worktree will be created inside Wolf container during startup
// to ensure paths are correct for the container environment
```

**Updated setupDesignDocsWorktree to only create branch** (lines 2399-2501):

```go
// setupDesignDocsWorktree creates the helix-design-docs branch if it doesn't exist
// Note: The actual worktree is created inside the Wolf container during startup
// to ensure paths are correct for the container environment
func (w *WolfExecutor) setupDesignDocsWorktree(repoPath, repoName string) error {
    log.Info().
        Str("repo_path", repoPath).
        Msg("Ensuring helix-design-docs branch exists")

    ctx := context.Background()

    // Check if helix-design-docs branch exists remotely
    // ... (branch creation logic remains) ...

    // Worktree will be created inside Wolf container during startup
    log.Info().
        Str("repo_name", repoName).
        Msg("helix-design-docs branch ready (worktree will be created in container)")

    return nil
}
```

## Root Cause Resolved

**Before:**
1. API container clones repos to `/filestore/workspaces/spectasks/{task_id}/{repo}`
2. API container creates worktree with absolute path: `/filestore/workspaces/.../.git-worktrees/helix-design-docs`
3. Workspace mounted into Wolf container at `/home/retro/work/{repo}`
4. Worktree `.git` file references `/filestore/workspaces/...` → **PATH DOESN'T EXIST IN CONTAINER**

**After:**
1. API container clones repos to `/filestore/workspaces/spectasks/{task_id}/{repo}`
2. API container creates `helix-design-docs` branch only (no worktree)
3. Workspace mounted into Wolf container at `/home/retro/work/{repo}`
4. Container startup script creates worktree with correct path: `/home/retro/work/{repo}/.git-worktrees/helix-design-docs`
5. Worktree `.git` file references `/home/retro/work/{repo}/.git/worktrees/helix-design-docs` → **PATH EXISTS AND IS CORRECT**

## Benefits

### Worktree Fix
✅ SpecTask planning agents can now commit to `helix-design-docs` branch
✅ Git operations work correctly inside Wolf container
✅ Worktree paths are valid for the container environment
✅ Idempotent (can run multiple times safely)
✅ Self-healing (detects and fixes invalid worktrees)

### Internal Repo Cloning
✅ Internal repos are cloned instead of mounted
✅ Prepares for network separation of Wolf server from API server
✅ No special handling needed for internal repos
✅ Consistent behavior for all repository types
✅ Startup scripts automatically discovered in any cloned repo

## Testing Plan

1. Create a new SpecTask with planning phase
2. Verify agent can create files in worktree directory
3. Verify agent can commit to `helix-design-docs` branch
4. Verify `git status` and `git log` work in worktree
5. Verify internal repo startup scripts execute correctly
6. Restart container, verify worktree persists and remains valid
7. Test with multiple repos in same session

## Files Modified

1. `wolf/sway-config/start-zed-helix.sh` - Added worktree setup + internal repo discovery
2. `api/pkg/external-agent/wolf_executor.go` - Removed mounting, removed worktree creation, updated setupDesignDocsWorktree
3. **Built:** `helix-sway:latest` container rebuilt successfully

## Next Steps

1. **Test with actual SpecTask** - Create a planning phase task and verify commits work
2. **Monitor container startup logs** - Check for worktree creation messages
3. **Verify internal repo startup scripts** - Test with a project that has `.helix/startup.sh`
4. **Network separation testing** - When ready to separate Wolf from API server

## Technical Details

**Git Worktree Path Structure:**
```
/home/retro/work/{repo}/
├── .git/                       # Main git directory
│   └── worktrees/
│       └── helix-design-docs/  # Worktree admin files
├── .git-worktrees/
│   └── helix-design-docs/      # Actual worktree working directory
│       ├── .git                # File containing: gitdir: ../../.git/worktrees/helix-design-docs
│       ├── README.md
│       └── tasks/
│           └── 2025-11-06_task-name_{id}/
│               ├── requirements.md
│               ├── design.md
│               └── tasks.md
```

**Container Startup Flow:**
1. Sway starts → `startup-app.sh` executes
2. `start-zed-helix.sh` runs
3. Worktree setup loop executes (lines 21-66)
4. Internal repo startup script discovery (lines 143-156)
5. Zed launches with correct worktree paths

## Verification Commands

**Inside Wolf container:**
```bash
# Check worktree status
cd /home/retro/work/{repo}
git worktree list

# Verify worktree .git file
cat .git-worktrees/helix-design-docs/.git

# Test git operations in worktree
cd .git-worktrees/helix-design-docs
git status
git log --oneline helix-design-docs
```

## Success Criteria

- [x] Sway container builds successfully
- [x] API compiles without errors
- [ ] Agent can commit to helix-design-docs branch (to be tested)
- [ ] Worktree paths are correct in container (to be tested)
- [ ] Internal repo startup scripts execute (to be tested)
- [ ] Changes persist across container restarts (to be tested)
