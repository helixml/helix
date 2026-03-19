# Design: Fix HELIX_PRIMARY_REPO_NAME Incorrect Value

## Architecture Overview

The primary repo flows: `project.DefaultRepoID` → `agent.PrimaryRepositoryID` → `HELIX_PRIMARY_REPO_NAME` env var.

```
project_handlers.go / spec_driven_task_service.go
    └─ construct DesktopAgent{PrimaryRepositoryID: ...}
           └─ hydra_executor.go StartDesktop()
                  └─ env: HELIX_PRIMARY_REPO_NAME=<repo.Name>
```

## The Fix

**File:** `api/pkg/server/session_handlers.go` (~lines 1868-1872)

Replace the unconditional `projectRepos[0]` assignment with the same pattern used everywhere else:

```go
// BEFORE (buggy):
if len(projectRepos) > 0 {
    agent.PrimaryRepositoryID = projectRepos[0].ID
}

// AFTER (consistent with spec_driven_task_service.go and project_handlers.go):
primaryRepoID := project.DefaultRepoID
if primaryRepoID == "" && len(projectRepos) > 0 {
    primaryRepoID = projectRepos[0].ID
}
if primaryRepoID != "" {
    agent.PrimaryRepositoryID = primaryRepoID
}
```

Note: `project` must be in scope at this point in session_handlers.go. If it isn't fetched yet, it needs to be retrieved from the store using the session's ProjectID.

## Implementation Notes

- `GetProject` was already called at line 1850 but its return value was discarded with `_`. Simply capturing it as `project` was sufficient — no extra store call needed.
- The fix is 6 lines changed (net +1 line): capture project return value, extract `primaryRepoID` from `DefaultRepoID`, fall back to `[0]` only when empty.
- Build: `go build ./pkg/server/` passes cleanly.

## Secondary Issue (Optional Improvement)

`ListGitRepositories` orders by `git_repositories.created_at DESC`. This means the fallback `[0]` is whichever repo was most recently inserted into the `git_repositories` table — not necessarily the most important one. This is confusing.

Consider ordering by `project_repositories.created_at ASC` (when the repo was attached to the project) or making the fallback deterministic in some other way. However, this is cosmetic once Bug 1 is fixed, since `DefaultRepoID` will be consulted first.

## Codebase Patterns Discovered

- **Primary repo field**: `project.DefaultRepoID` (not `is_primary` flag) — set via `PUT /api/v1/projects/{id}/repositories/{repo_id}/primary`
- **Repo list query**: Junction table `project_repositories` enforces per-project isolation; no cross-project contamination risk
- **Three call sites** for `StartDesktop()`: `spec_driven_task_service.go`, `project_handlers.go`, `session_handlers.go` — only the last is buggy
- **HELIX_REPOSITORIES** env var prefix (e.g., `prj_01kgb5y03prmjd5z4hykxze512-docs`) reflects the repo's original owner project, not the current project
