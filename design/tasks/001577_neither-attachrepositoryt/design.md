# Design: Fix default_repo_id Sync on Attach/Detach

## Architecture

All changes are in the Go backend. No frontend changes are needed — the bug is purely a data consistency issue.

### Relevant Files

| File | Role |
|------|------|
| `api/pkg/server/project_handlers.go` | HTTP handlers for attach/detach routes |
| `api/pkg/store/postgres.go` | `SetProjectPrimaryRepository`, `AttachRepositoryToProject`, `DetachRepositoryFromProject` store methods |
| `api/pkg/store/project_repository.go` | `GetRepositoriesForProject`, `DeleteProjectRepository` |
| `api/pkg/types/project.go` | `Project.DefaultRepoID` field |

### Existing Helper

`store.SetProjectPrimaryRepository(ctx, projectID, repoID)` already exists and sets `default_repo_id` via a targeted UPDATE. Use this rather than writing new SQL.

## Changes

### 1. `attachRepositoryToProject` handler (`project_handlers.go`)

After the successful `s.Store.AttachRepositoryToProject(projectID, repoID)` call, check whether `project.DefaultRepoID` is empty or no longer in the attached set:

```
currentRepos = s.Store.GetRepositoriesForProject(projectID)  // includes the newly attached one
if project.DefaultRepoID == "" || not in currentRepos {
    s.Store.SetProjectPrimaryRepository(ctx, projectID, repoID)
}
```

### 2. `detachRepositoryFromProject` handler (`project_handlers.go`)

After the successful `s.Store.DetachRepositoryFromProject(projectID, repoID)` call, check whether the detached repo was the default:

```
if project.DefaultRepoID == repoID {
    remainingRepos = s.Store.GetRepositoriesForProject(projectID)  // after detach
    if len(remainingRepos) > 0 {
        s.Store.SetProjectPrimaryRepository(ctx, projectID, remainingRepos[0])
    } else {
        s.Store.SetProjectPrimaryRepository(ctx, projectID, "")  // or direct UPDATE to ""
    }
}
```

> **Note:** `SetProjectPrimaryRepository` may not support empty string — check the implementation. If it doesn't, use a direct `gdb.Model(&Project{}).Where("id = ?", projectID).Update("default_repo_id", "")` call or add a store helper.

## Decisions

- **Handler-level vs store-level:** Changes go in the HTTP handler layer because the logic involves cross-entity reads (project + repositories) which the store methods don't currently encapsulate. Keeping it in handlers is consistent with the existing pattern.
- **Which repo becomes new default on detach:** Pick `remainingRepos[0]` (arbitrary but deterministic). No ordering requirement specified.
- **No frontend changes:** The frontend already invalidates project + repository queries on attach/detach actions; once the backend returns the correct `default_repo_id`, `hasExternalRepo` will be correct automatically.

## Discovered Patterns

- The project handler file follows a read-validate-mutate pattern: get entity → check auth → validate business rules → call store → return response.
- `GetRepositoriesForProject` returns `[]string` (repository IDs), not full objects. The detach logic only needs IDs so this is sufficient.
- `AttachRepositoryToProject` and `DetachRepositoryFromProject` in the store handle only the junction table (`project_repositories`); they do not touch the `projects` table at all.
