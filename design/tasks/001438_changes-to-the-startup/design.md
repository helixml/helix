# Design: Startup Script Not Persisted on Desktop Restart

## Overview

Fix the bug where startup script changes saved via the UI don't take effect when restarting the exploratory session desktop.

## Architecture

```
┌─────────────────────┐     ┌──────────────────────┐     ┌─────────────────────┐
│  Project Settings   │────▶│  API: updateProject  │────▶│  Bare Git Repo      │
│  (Frontend)         │     │  SaveStartupScript   │     │  helix-specs branch │
└─────────────────────┘     │  ToHelixSpecs()      │     └─────────────────────┘
                            └──────────────────────┘              │
                                                                  │ (push)
                                                                  ▼
┌─────────────────────┐     ┌──────────────────────┐     ┌─────────────────────┐
│  Desktop Container  │◀────│  helix-workspace-    │◀────│  origin/helix-specs │
│  runs startup.sh    │     │  setup.sh            │     │  (remote)           │
└─────────────────────┘     └──────────────────────┘     └─────────────────────┘
                                    │
                                    │ BUG: doesn't pull
                                    │ when worktree exists
                                    ▼
                            ┌──────────────────────┐
                            │  ~/work/helix-specs  │
                            │  (git worktree)      │
                            └──────────────────────┘
```

## Current Behavior (Bug)

In `helix-workspace-setup.sh` lines 460-470:
```bash
else
    echo "  Design docs worktree already exists"
    CURRENT_BRANCH=$(git -C "$WORKTREE_PATH" branch --show-current 2>/dev/null || echo "unknown")
    echo "  Current branch: $CURRENT_BRANCH"
    # ... creates task dir but NEVER PULLS
fi
```

## Solution

Add `git fetch` + `git pull` to the existing worktree case in `helix-workspace-setup.sh`.

### Key Design Decisions

1. **Fetch + pull, not reset**: Use `git pull` to merge remote changes rather than `git reset --hard` to preserve any local uncommitted work (e.g., agent mid-task changes).

2. **Stash local changes**: If the worktree has uncommitted changes, stash them before pulling, then re-apply. This prevents merge conflicts from blocking the pull.

3. **Non-fatal errors**: If pull fails (network issues, merge conflict), log a warning but continue. The desktop should still start - just with potentially stale startup script.

4. **Parallel with main repo fetch**: The existing code already fetches the main repo at line 325. The worktree shares the same git database, so its fetch is incremental.

## Code Change Location

**File**: `helix/desktop/shared/helix-workspace-setup.sh`
**Section**: Lines 460-470 (existing worktree case)

```bash
else
    echo "  Design docs worktree already exists"
    CURRENT_BRANCH=$(git -C "$WORKTREE_PATH" branch --show-current 2>/dev/null || echo "unknown")
    echo "  Current branch: $CURRENT_BRANCH"
    
    # NEW: Pull latest from remote to get updated startup script
    echo "  Pulling latest changes from remote..."
    HELIX_SPECS_STASHED=false
    if ! git -C "$WORKTREE_PATH" diff --quiet 2>/dev/null; then
        echo "  Stashing local changes..."
        git -C "$WORKTREE_PATH" stash push -m "helix-workspace-setup" 2>&1 && HELIX_SPECS_STASHED=true
    fi
    
    if git -C "$WORKTREE_PATH" pull origin helix-specs 2>&1; then
        echo "  ✅ helix-specs updated"
    else
        echo "  ⚠️ Failed to pull helix-specs (continuing with local version)"
    fi
    
    if [ "$HELIX_SPECS_STASHED" = true ]; then
        git -C "$WORKTREE_PATH" stash pop 2>&1 || echo "  ⚠️ Failed to restore stashed changes"
    fi
    
    # ... existing task dir creation
fi
```

## Testing

1. Create a project with a startup script that echoes "version 1"
2. Start exploratory session, verify "version 1" appears in terminal
3. Edit startup script to echo "version 2" in Project Settings
4. Click "Test Startup Script" (restarts desktop)
5. Verify "version 2" appears in terminal

## Risks

- **Merge conflicts**: If agent made local changes to helix-specs that conflict with remote, pull will fail. Mitigated by stash + warning log.
- **Network latency**: Pull adds ~1-2 seconds to restart. Acceptable for correctness.