# Fix SpecTask Design Docs Worktree Path Mismatch

**Date:** 2025-11-06
**Status:** Bug Identified - Fix Proposed
**Priority:** Critical (blocks SpecTask planning agents)

## Problem Statement

SpecTask planning agents running in Wolf containers cannot commit design documents to the `helix-design-docs` branch because the git worktree has invalid paths.

### Observed Symptoms (from user transcript)

```bash
# Inside Wolf container at /home/retro/work/modern-todo-app/.git-worktrees/helix-design-docs
$ cat .git
gitdir: /filestore/workspaces/spec-tasks/{id}/modern-todo-app/.git/worktrees/helix-design-docs

$ git status
fatal: not a git repository: /filestore/workspaces/spec-tasks/{id}/modern-todo-app/.git/worktrees/helix-design-docs
```

The `.git` file points to `/filestore/workspaces/...` which doesn't exist inside the Wolf container!

## Root Cause Analysis

### Current Flow (Broken)

1. **API Container** (at session start):
   - Clones repositories to: `/filestore/workspaces/spectasks/{task_id}/{repo_name}`
   - Calls `setupDesignDocsWorktree(cloneDir, repo.Name)` where `cloneDir` is the API container path
   - Executes: `git worktree add /filestore/workspaces/spectasks/{task_id}/{repo}/.git-worktrees/helix-design-docs helix-design-docs`
   - **Result**: Worktree created with API container absolute paths

2. **Wolf Container** mounts workspace:
   - Workspace mounted at: `/home/retro/work/{repo_name}`
   - Agent tries to use worktree at: `/home/retro/work/{repo_name}/.git-worktrees/helix-design-docs`
   - `.git` file contains: `gitdir: /filestore/workspaces/spectasks/{task_id}/{repo}/.git/worktrees/helix-design-docs`
   - **Result**: Path doesn't exist → `fatal: not a git repository`

### Why This Happens

Git worktrees use **absolute paths** in the `.git` file. When the worktree is created in the API container, git writes the API container's absolute path. When the filesystem is mounted into the Wolf container at a different location, those absolute paths break.

**Relevant Code:**
- `api/pkg/external-agent/wolf_executor.go:2310` - Creates `cloneDir` with API container path
- `api/pkg/external-agent/wolf_executor.go:2321, 2424` - Calls `setupDesignDocsWorktree(cloneDir, ...)`
- `api/pkg/external-agent/wolf_executor.go:2546` - Executes `git worktree add {worktreePath} helix-design-docs` in API container
- `api/pkg/external-agent/wolf_executor.go:2561-2566` - `execCommand` runs in API container context

## Solution: Create Worktree Inside Wolf Container

### Approach 1: Move Worktree Creation to Container Startup ✅ RECOMMENDED

**Change:** Create the worktree inside the Wolf container after the workspace is mounted.

**Location:** `wolf/sway-config/start-zed-helix.sh` (runs inside Wolf container)

**Add after line 19 (after `cd $WORK_DIR`):**

```bash
# Setup helix-design-docs worktree for SpecTask repositories
# This must happen INSIDE the Wolf container so paths are correct
echo "Setting up design docs worktrees..."
for repo_dir in */; do
    # Check if this is a git repository
    if [ -d "$repo_dir/.git" ]; then
        repo_name="${repo_dir%/}"
        worktree_path="$repo_dir/.git-worktrees/helix-design-docs"

        # Check if helix-design-docs branch exists
        if git -C "$repo_dir" rev-parse --verify helix-design-docs >/dev/null 2>&1; then
            # Branch exists, check if worktree is already set up
            if [ ! -d "$worktree_path" ]; then
                echo "  Creating worktree for $repo_name..."
                git -C "$repo_dir" worktree add "$worktree_path" helix-design-docs 2>/dev/null || {
                    echo "  ⚠️  Worktree creation failed for $repo_name (may already exist)"
                }
            else
                echo "  ✅ Worktree exists for $repo_name"
            fi
        fi
    fi
done
echo "Design docs worktrees ready"
```

**Changes to wolf_executor.go:**

```diff
-			// For primary repository, ensure design docs worktree exists
-			if repoID == primaryRepositoryID {
-				err := w.setupDesignDocsWorktree(cloneDir, repo.Name)
-				if err != nil {
-					log.Error().Err(err).Msg("Failed to setup design docs worktree for existing repo")
-					// Continue - not fatal
-				}
-			}

+			// Note: Design docs worktree will be created inside Wolf container during startup
+			// to ensure paths are correct for the container environment
```

```diff
-		// For primary repository, setup design docs worktree
-		if repoID == primaryRepositoryID {
-			err := w.setupDesignDocsWorktree(cloneDir, repo.Name)
-			if err != nil {
-				log.Error().Err(err).Msg("Failed to setup design docs worktree")
-				// Continue - not fatal, agent can manually create
-			}
-		}

+		// Note: Design docs worktree will be created inside Wolf container during startup
+		// to ensure paths are correct for the container environment
```

**Keep setupDesignDocsWorktree for branch creation only:**

The branch creation logic (lines 2454-2538) is fine to run in the API container since it only creates refs, doesn't set up worktrees. Just remove the worktree creation part (lines 2540-2557).

### Approach 2: Use Relative Paths in Worktree ❌ NOT RECOMMENDED

Git worktrees don't support relative paths in the `.git` file. This would require patching git itself.

### Approach 3: Symlink Worktree Directory ❌ FRAGILE

Create a symlink inside the container from `/home/retro/work` → `/filestore/workspaces/...`. This is fragile and depends on the mount structure.

## Implementation Plan

### Phase 1: Add Worktree Setup to Container Startup

1. **Edit `wolf/sway-config/start-zed-helix.sh`**:
   - Add worktree setup logic after line 19 (see code above)
   - Test with a SpecTask that has repositories

2. **Remove worktree creation from API container**:
   - Edit `api/pkg/external-agent/wolf_executor.go:2321` - Remove worktree setup call
   - Edit `api/pkg/external-agent/wolf_executor.go:2424` - Remove worktree setup call
   - Keep branch creation logic (lines 2454-2538)
   - Remove worktree add logic (lines 2540-2557)

3. **Update setupDesignDocsWorktree function**:
   - Keep branch creation logic only
   - Remove worktree creation entirely
   - Document that worktree is created in container

### Phase 2: Test

1. Create a new SpecTask with planning phase
2. Verify design docs branch is created
3. Verify worktree is created inside Wolf container
4. Verify agent can commit to worktree
5. Verify git operations work correctly

### Phase 3: Cleanup

1. Remove unused `setupDesignDocsWorktree` worktree code
2. Add logging to container startup script
3. Update documentation

## Benefits of This Approach

1. **Correct paths**: Worktree paths match the container environment
2. **Atomic setup**: Worktree is created before Zed starts
3. **Idempotent**: Script can run multiple times safely
4. **Automatic**: No manual intervention needed
5. **Resilient**: Works across container restarts

## Testing Checklist

- [ ] Create new SpecTask with planning phase
- [ ] Verify helix-design-docs branch exists
- [ ] Verify worktree directory exists at `.git-worktrees/helix-design-docs`
- [ ] Verify `.git` file has correct path
- [ ] Verify agent can navigate to worktree directory
- [ ] Verify agent can create files in worktree
- [ ] Verify agent can commit to helix-design-docs branch
- [ ] Verify git log shows commits
- [ ] Restart Wolf container, verify worktree still works

## Alternative: User Startup Script Approach

Instead of modifying `start-zed-helix.sh`, we could generate a `startup.sh` in the workspace that sets up worktrees. This would be written by wolf_executor.go during repository setup.

**Pros:**
- Keeps container scripts generic
- Can be customized per task

**Cons:**
- More complex (need to generate script content)
- Relies on `HELIX_STARTUP_SCRIPT` mechanism (not currently implemented in our scripts)

For simplicity, **Approach 1** (modifying start-zed-helix.sh) is recommended.

## Files to Modify

1. `wolf/sway-config/start-zed-helix.sh` - Add worktree setup
2. `api/pkg/external-agent/wolf_executor.go` - Remove worktree creation calls
3. `api/pkg/external-agent/wolf_executor.go` - Update `setupDesignDocsWorktree` function

## Related Issues

- User transcript shows this exact problem in a modern-todo-app test
- Affects ALL SpecTasks with planning phases
- Critical blocker for design document workflow

## Success Criteria

✅ SpecTask planning agents can commit to helix-design-docs branch
✅ Git operations work correctly in Wolf container
✅ Worktree paths are valid inside container
✅ No manual intervention required
✅ Works across container restarts
