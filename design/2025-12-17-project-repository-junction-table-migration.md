# Project Repository Junction Table Migration

**Date:** 2025-12-17
**Status:** Implemented
**Author:** Claude
**Risk Level:** High - Database schema change affecting core entity relationships

## Problem Statement

When attaching a repository as a primary repo to a project, it gets detached from any other project where it was previously attached as a primary repo. This is because the `GitRepository` table has a single `project_id` field, which can only hold ONE project ID at a time.

**Bug Flow:**
1. Repo X attached to Project A: `repo_x.project_id = "project-a"`
2. Repo X attached to Project B: `repo_x.project_id = "project-b"` (overwrites!)
3. Project A's repo list (`WHERE project_id = 'project-a'`) no longer returns Repo X

## Solution

Created a `project_repositories` junction table to support many-to-many relationships between projects and repositories.

### New Data Model

```
ProjectRepository (NEW junction table)
├── project_id: string (PK, composite)
├── repository_id: string (PK, composite)
├── organization_id: string (index, for access control)
├── created_at: timestamp
└── updated_at: timestamp

GitRepository (MODIFIED)
├── project_id: DEPRECATED (kept for backward compatibility)
└── ... (other fields unchanged)

Project (UNCHANGED)
├── default_repo_id: string (references primary repo)
└── ...
```

## Design Decisions

1. **Push behavior**: When a git push happens to a repo attached to multiple projects, SpecTask updates are triggered for ALL projects that have the repo attached.

2. **Cascade delete**: Junction table entries are automatically deleted when a project or repository is deleted (via FK constraints).

3. **Database backward compatibility**: The `project_id` column is NEVER dropped. All writes go to BOTH the junction table AND the legacy column. This allows rollback to older code versions.

4. **API backward compatibility**: The `project_id` field remains in API responses, populated from the legacy database column.

## Files Modified

| File | Change |
|------|--------|
| `api/pkg/types/project_repository.go` | NEW - Junction table type |
| `api/pkg/store/project_repository.go` | NEW - CRUD operations + migration |
| `api/pkg/store/store.go` | Added interface methods |
| `api/pkg/store/postgres.go` | AutoMigrate, FKs, updated attach/detach |
| `api/pkg/store/git_repository.go` | Updated ListGitRepositories with JOIN |
| `api/pkg/types/git_repositories.go` | Added deprecation comment |
| `api/pkg/server/project_handlers.go` | Updated detach call signature |
| `api/pkg/services/git_http_server.go` | Loop through all projects on push |
| `api/pkg/services/git_repository_service.go` | Attach after create |
| `api/pkg/server/simple_sample_projects.go` | Attach after each repo create |
| `api/pkg/store/store_mocks.go` | Regenerated |

## Key Implementation Details

### AttachRepositoryToProject

Writes to BOTH junction table and legacy project_id column:
```go
// Write to junction table (idempotent)
s.CreateProjectRepository(ctx, projectID, repoID, repo.OrganizationID)
// Also write to legacy column for rollback compatibility
s.gdb.Model(&GitRepository{}).Where("id = ?", repoID).Update("project_id", projectID)
```

### DetachRepositoryFromProject

Signature changed to require projectID (since repos can be attached to multiple projects):
```go
// OLD: DetachRepositoryFromProject(ctx, repoID)
// NEW: DetachRepositoryFromProject(ctx, projectID, repoID)
```

### ListGitRepositories

Uses JOIN on junction table:
```go
if request.ProjectID != "" {
    query = query.Joins("INNER JOIN project_repositories ON ...").
        Where("project_repositories.project_id = ?", request.ProjectID)
}
```

### Git Push Handling

Loops through ALL projects that have the repo attached:
```go
projectIDs, _ := s.store.GetProjectsForRepository(ctx, repo.ID)
for _, projectID := range projectIDs {
    tasks, _ := s.store.ListSpecTasks(ctx, &types.SpecTaskFilters{ProjectID: projectID})
    // Process tasks...
}
```

## Rollback Plan

**Database backward compatibility is maintained:**
1. `project_id` column is NEVER dropped from `git_repositories` table
2. All writes go to BOTH junction table AND project_id column
3. To rollback: simply deploy older code version
   - Old code reads from `project_id` column (still populated)
   - Junction table is ignored by old code (no harm)
4. No data migration needed for rollback

## Testing Checklist

- [ ] Create two projects
- [ ] Attach same repo to both as primary
- [ ] Verify both projects show repo in list
- [ ] Push to repo - verify SpecTasks updated for both projects
- [ ] Detach from one project - verify other still has it
- [ ] Delete project - verify junction entries cleaned up
- [ ] Delete repo - verify junction entries cleaned up
