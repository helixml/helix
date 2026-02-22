# Design: Auto-Initialize Empty Git Repositories

## Overview

Modify `helix-workspace-setup.sh` to detect empty repositories after cloning and automatically initialize them with an initial commit before attempting branch operations.

## Git Proxy Layer Context

Helix proxies all git operations through its API server. The relevant code paths:

- **`api/pkg/services/git_external_sync.go`**: `WithExternalRepoRead` / `WithExternalRepoWrite` - wrappers that sync with upstream before operations
- **`api/pkg/services/git_repository_service.go`**: `updateRepositoryFromGit` uses `giteagit.Clone` with `Mirror: true`
- **`api/pkg/services/git_integration_test.go`**: `TestSyncAllBranches_EmptyRepo` - explicit test coverage for empty repos

**The git proxy already handles empty repos correctly.** The clone via `giteagit.Clone(Mirror: true)` succeeds for empty repos. The problem is downstream in the desktop shell script that runs inside the container after the clone completes.

## Current Architecture

```
API git proxy (Clone with Mirror:true) → Desktop container receives repo → helix-workspace-setup.sh
                                                                                    ↓
                                                                           Fetch branches → Find base branch → FAILS
```

## Proposed Solution

```
Clone repo → Detect empty repo → Initialize if needed → Fetch branches → Continue normally
                   ↓
            Create initial commit + push main branch
```

## Key Design Decisions

### Decision 1: Where to add the fix

**Location:** In `helix-workspace-setup.sh`, right after the clone completes and before "Configuring branch..." section.

**Rationale:** 
- `helix-specs-create.sh` already handles empty repos but runs later
- We need to initialize BEFORE any branch checkout attempts
- Keep the fix close to where the problem occurs

### Decision 2: Detection method

**Approach:** Check if the repo has any commits using `git rev-parse HEAD`

```bash
# Returns exit code 128 if no commits exist
if ! git -C "$REPO_PATH" rev-parse HEAD >/dev/null 2>&1; then
    # Repo is empty - needs initialization
fi
```

**Alternative considered:** Check for remote branches with `git branch -r`
**Why rejected:** Less reliable, doesn't distinguish between "no branches" and "fetch failed"

### Decision 3: Initial commit content

**Approach:** Minimal README with repo name
```
# {repo-name}

This repository was initialized by Helix.
```

**Rationale:** 
- Users expect to see something in an initialized repo
- Minimal content doesn't impose opinions on project structure
- README is conventional for any project

## Implementation Details

### Location in helix-workspace-setup.sh

Insert new section between:
- After: Clone completion (around line 250, after "Waiting for clones to complete")
- Before: "Configuring branch..." section (around line 270)

### Pseudo-code

```bash
# =========================================
# Initialize empty repositories
# =========================================
for CLONE_DIR in "${CLONE_DIRS[@]}"; do
    if [ -d "$CLONE_DIR/.git" ]; then
        if ! git -C "$CLONE_DIR" rev-parse HEAD >/dev/null 2>&1; then
            REPO_NAME=$(basename "$CLONE_DIR")
            echo "  Initializing empty repository: $REPO_NAME"
            
            # Create README
            echo "# $REPO_NAME" > "$CLONE_DIR/README.md"
            echo "" >> "$CLONE_DIR/README.md"
            echo "This repository was initialized by Helix." >> "$CLONE_DIR/README.md"
            
            # Commit and push
            git -C "$CLONE_DIR" add README.md
            git -C "$CLONE_DIR" commit -m "Initial commit"
            git -C "$CLONE_DIR" push -u origin main
            
            echo "  ✅ Repository initialized with main branch"
        fi
    fi
done
```

## Edge Cases

| Case | Handling |
|------|----------|
| Push fails (permissions) | Let it fail with clear error - user needs to fix repo permissions |
| Multiple empty repos | Initialize each independently |
| Repo becomes non-empty between clone and init | Check prevents double-init |

## Testing

Manual test:
1. Create empty repo on GitHub
2. Start Helix session pointing to it
3. Verify: No error, Zed opens, repo has initial commit

## Implementation Notes

### What was actually implemented

The clone in `helix-workspace-setup.sh` is a regular working directory clone (not bare, not mirror), so the implementation is simpler than originally designed:

1. Loop through `CLONE_DIRS` array after clone completion
2. Check for empty repos with `git rev-parse HEAD`
3. Create README.md directly in the working directory
4. Checkout `main` branch, commit, and push

### Key differences from design

- **No temp directory needed**: The cloned repo is a working directory, not bare
- **Used `git checkout -b main`**: Creates the main branch explicitly before committing
- **Single branch attempt**: Only try `main` (not fallback to `master`) since we're creating fresh

### Files modified

- `desktop/shared/helix-workspace-setup.sh`: Added empty repo initialization section at lines 255-293

### Git user config

Git user.name and user.email are already configured globally earlier in the script (lines 139-154), so no additional config needed.