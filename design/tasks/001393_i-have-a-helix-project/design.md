# Design: Primary Repository Ordering Fix

## Root Cause Analysis

### Issue 1: UI displays repos in wrong order

**Location:** `api/pkg/store/git_repository.go:73`

```go
err := query.Order("git_repositories.created_at DESC").Find(&repos).Error
```

The `ListGitRepositories` function orders repos by `created_at DESC` (newest first). There's no special handling for the primary repository.

### Issue 2: Startup script already works correctly

After investigation, the startup script (`desktop/shared/helix-workspace-setup.sh`) **does** correctly change to the primary repo before running the startup script (lines 743-747):

```bash
if [ -n "$HELIX_PRIMARY_REPO_NAME" ] && [ -d "$WORK_DIR/$HELIX_PRIMARY_REPO_NAME" ]; then
    cd "$WORK_DIR/$HELIX_PRIMARY_REPO_NAME"
    echo "Working directory: $HELIX_PRIMARY_REPO_NAME"
fi
```

The `HELIX_PRIMARY_REPO_NAME` env var is correctly set in `api/pkg/external-agent/hydra_executor.go:273-287`.

**The reported bug may be user confusion** from the UI showing repos in the wrong order, making it appear the first-listed repo is used.

## Solution

### Fix 1: Sort repos with primary first in API response

Modify `getProjectRepositories` handler in `api/pkg/server/project_handlers.go` to sort results with primary repo first.

**Approach:** After fetching repos, reorder the slice to put the primary repo (matching `project.DefaultRepoID`) at index 0.

```go
// After fetching repos, sort with primary first
if project.DefaultRepoID != "" {
    sort.SliceStable(repos, func(i, j int) bool {
        return repos[i].ID == project.DefaultRepoID
    })
}
```

### Alternative: Frontend sorting

Could sort in `ProjectRepositoriesList.tsx`, but backend is preferred because:
1. All API consumers get consistent ordering
2. Single source of truth
3. Frontend already has `primaryRepoId` prop but doesn't use it for sorting

## Data Flow

```
Project.DefaultRepoID (DB)
    ↓
getProjectRepositories() → sorts with primary first
    ↓
Frontend displays in received order
```

## Files to Modify

| File | Change |
|------|--------|
| `api/pkg/server/project_handlers.go` | Sort repos with primary first in `getProjectRepositories` |

## Testing

1. Create project with 3+ repos, set middle one as primary
2. Verify API returns primary repo first
3. Verify UI shows primary repo at top of list
4. Verify startup script still runs in primary repo directory (regression test)