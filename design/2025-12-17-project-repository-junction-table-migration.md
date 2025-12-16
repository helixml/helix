# Project Repository Junction Table Migration

**Date:** 2025-12-17
**Status:** Planning
**Author:** Claude
**Risk Level:** High - Database schema change affecting core entity relationships

## Problem Statement

When attaching a repository as a primary repo to a project, it gets detached from any other project where it was previously attached as a primary repo. This is because the `GitRepository` table has a single `project_id` field, which can only hold ONE project ID at a time.

**Current Data Model (Broken):**
```
GitRepository
├── project_id: string  ← Can only reference ONE project
└── ...

Project
├── default_repo_id: string  ← References the primary repo
└── ...
```

**Bug Flow:**
1. Repo X attached to Project A → `repo_x.project_id = "project-a"`
2. Repo X attached to Project B → `repo_x.project_id = "project-b"` (overwrites!)
3. Project A's repo list (`WHERE project_id = 'project-a'`) no longer returns Repo X

## Solution: Junction Table

Replace the single `project_id` field with a many-to-many junction table `project_repositories`.

**New Data Model:**
```
ProjectRepository (NEW junction table)
├── project_id: string (PK, composite)
├── repository_id: string (PK, composite)
├── organization_id: string (index, for access control)
├── created_at: timestamp
└── updated_at: timestamp

GitRepository (MODIFIED)
├── project_id: REMOVED
└── ... (other fields unchanged)

Project (UNCHANGED)
├── default_repo_id: string  ← Still references the primary repo
└── ...
```

## Backward Compatibility Requirements

1. **API Compatibility**: Existing API endpoints must continue to work
   - `PUT /api/v1/projects/{id}/repositories/{repo_id}/attach` - Still works
   - `PUT /api/v1/projects/{id}/repositories/{repo_id}/detach` - Still works
   - `GET /api/v1/projects/{id}/repositories` - Still returns repos for project

2. **Data Migration**: Existing `project_id` values must be migrated to junction table
   - All repos with non-empty `project_id` get a junction table entry
   - No data loss during migration

3. **Frontend Compatibility**: TypeScript interfaces updated, but behavior unchanged

## Files Requiring Changes

### Phase 1: Add Junction Table (Additive - No Breaking Changes)

| File | Change |
|------|--------|
| `api/pkg/types/project_repository.go` | NEW - Junction table type definition |
| `api/pkg/store/project_repository.go` | NEW - CRUD operations for junction table |
| `api/pkg/store/store.go` | Add interface methods for junction table |
| `api/pkg/store/postgres.go` | Add to AutoMigrate list |

### Phase 2: Data Migration

| File | Change |
|------|--------|
| `api/pkg/store/postgres.go` | Add migration function to copy project_id → junction table |

### Phase 3: Update Store Layer to Use Junction Table

| File | Change |
|------|--------|
| `api/pkg/store/git_repository.go` | Update `ListGitRepositories` to JOIN on junction table |
| `api/pkg/store/postgres.go` | Update `AttachRepositoryToProject` to insert junction record |
| `api/pkg/store/postgres.go` | Update `DetachRepositoryFromProject` to delete junction record |
| `api/pkg/store/store_mocks.go` | Regenerate mocks |

### Phase 4: Update Service Layer

| File | Change |
|------|--------|
| `api/pkg/services/git_repository_service.go` | Remove `ProjectID` from create request handling |
| `api/pkg/services/git_http_server.go` | Update `repo.ProjectID` reads to use junction table |
| `api/pkg/services/spec_driven_task_service.go` | Update repo filtering logic |
| `api/pkg/services/spec_task_orchestrator.go` | Update repo listing |

### Phase 5: Update Handlers

| File | Change |
|------|--------|
| `api/pkg/server/project_handlers.go` | Update attach/detach handlers |
| `api/pkg/server/spec_task_clone_handlers.go` | Update clone logic |
| `api/pkg/server/simple_sample_projects.go` | Update sample project creation |

### Phase 6: Update Types and Remove project_id

| File | Change |
|------|--------|
| `api/pkg/types/git_repositories.go` | Remove `ProjectID` field from struct |
| `api/pkg/types/git_repositories.go` | Remove `ProjectID` from `GitRepositoryCreateRequest` |
| Frontend TypeScript | Regenerate via `./stack update_openapi` |

## Detailed Implementation

### 1. Junction Table Type (`api/pkg/types/project_repository.go`)

```go
package types

import "time"

// ProjectRepository represents the many-to-many relationship between projects and repositories
type ProjectRepository struct {
    ProjectID      string `json:"project_id" gorm:"primaryKey"` // composite key
    RepositoryID   string `json:"repository_id" gorm:"primaryKey"`
    OrganizationID string `json:"organization_id" gorm:"index"` // For access control queries
    CreatedAt      time.Time `json:"created_at"`
    UpdatedAt      time.Time `json:"updated_at"`
}

// TableName overrides the table name
func (ProjectRepository) TableName() string {
    return "project_repositories"
}

// Query types
type ListProjectRepositoriesQuery struct {
    ProjectID      string
    RepositoryID   string
    OrganizationID string
}
```

### 2. Migration Strategy

**Step 1: Create junction table (AutoMigrate)**
- GORM AutoMigrate creates the new table
- Existing data untouched

**Step 2: Copy data from project_id to junction table**
```sql
INSERT INTO project_repositories (project_id, repository_id, organization_id, created_at, updated_at)
SELECT project_id, id, organization_id, NOW(), NOW()
FROM git_repositories
WHERE project_id IS NOT NULL AND project_id != ''
ON CONFLICT (project_id, repository_id) DO NOTHING;
```

**Step 3: Update code to use junction table**
- All reads/writes go through junction table
- project_id field is ignored

**Step 4: Remove project_id column (optional, can be deferred)**
- Only after confirming everything works
- Can leave column in place initially for safety

### 3. Updated Store Methods

**AttachRepositoryToProject (NEW behavior):**
```go
func (s *PostgresStore) AttachRepositoryToProject(ctx context.Context, projectID string, repoID string) error {
    // Get repo to find organization_id
    repo, err := s.GetGitRepository(ctx, repoID)
    if err != nil {
        return fmt.Errorf("failed to get repository: %w", err)
    }

    // Insert junction record (upsert to handle duplicates)
    junction := &types.ProjectRepository{
        ProjectID:      projectID,
        RepositoryID:   repoID,
        OrganizationID: repo.OrganizationID,
        CreatedAt:      time.Now(),
        UpdatedAt:      time.Now(),
    }

    err = s.gdb.WithContext(ctx).
        Clauses(clause.OnConflict{DoNothing: true}).
        Create(junction).Error
    if err != nil {
        return fmt.Errorf("failed to attach repository to project: %w", err)
    }
    return nil
}
```

**DetachRepositoryFromProject (NEW behavior):**
```go
func (s *PostgresStore) DetachRepositoryFromProject(ctx context.Context, projectID string, repoID string) error {
    err := s.gdb.WithContext(ctx).
        Where("project_id = ? AND repository_id = ?", projectID, repoID).
        Delete(&types.ProjectRepository{}).Error
    if err != nil {
        return fmt.Errorf("failed to detach repository from project: %w", err)
    }
    return nil
}
```

**ListGitRepositories (Updated to use junction table):**
```go
func (s *PostgresStore) ListGitRepositories(ctx context.Context, request *types.ListGitRepositoriesRequest) ([]*types.GitRepository, error) {
    var repos []*types.GitRepository
    query := s.gdb.WithContext(ctx)

    if request.OwnerID != "" {
        query = query.Where("owner_id = ?", request.OwnerID)
    }
    if request.OrganizationID != "" {
        query = query.Where("organization_id = ?", request.OrganizationID)
    }

    // NEW: Use junction table for project filtering
    if request.ProjectID != "" {
        query = query.Joins("INNER JOIN project_repositories ON project_repositories.repository_id = git_repositories.id").
            Where("project_repositories.project_id = ?", request.ProjectID)
    }

    err := query.Order("created_at DESC").Find(&repos).Error
    if err != nil {
        return nil, err
    }
    return repos, nil
}
```

### 4. API Endpoint Changes

**detachRepositoryFromProject handler needs projectID:**

Current signature:
```go
DetachRepositoryFromProject(ctx context.Context, repoID string) error
```

New signature (BREAKING for store interface):
```go
DetachRepositoryFromProject(ctx context.Context, projectID string, repoID string) error
```

This is needed because a repo can now be attached to multiple projects - we need to specify which project to detach from.

## Testing Plan

1. **Unit Tests:**
   - Junction table CRUD operations
   - Migration function
   - Updated ListGitRepositories with JOIN

2. **Integration Tests:**
   - Attach same repo to multiple projects
   - List repos for each project (should both show the repo)
   - Detach from one project (other should still have it)
   - Set as primary repo for multiple projects

3. **Manual Testing:**
   - Create two projects
   - Attach same repo to both as primary
   - Verify both projects show the repo in their list
   - Verify both projects can use the repo as primary

## Rollback Plan

If issues arise:
1. Junction table can be dropped (AutoMigrate won't remove it automatically)
2. Code can be reverted to use project_id field
3. project_id field is NOT removed until migration is confirmed successful

## Risk Assessment

| Risk | Mitigation |
|------|------------|
| Data loss during migration | Migration is additive; project_id preserved |
| Breaking existing API | API endpoints unchanged; only internal implementation changes |
| Performance regression | Added indexes on junction table; JOIN is efficient |
| Incomplete code update | Comprehensive file list above; grep for remaining usages |

## Implementation Order

1. Create junction table type and store methods
2. Add to AutoMigrate
3. Run migration to copy existing data
4. Update `ListGitRepositories` to use JOIN
5. Update `AttachRepositoryToProject` to use junction table
6. Update `DetachRepositoryFromProject` signature and implementation
7. Update all handlers and services
8. Remove `ProjectID` from GitRepository type
9. Regenerate OpenAPI/TypeScript
10. Test thoroughly
11. (Optional) Drop project_id column later

## Open Questions

1. Should we keep the `project_id` column indefinitely as a denormalized cache, or remove it entirely?
   - **Recommendation:** Remove it to avoid confusion, but only after confirming migration success

2. Should `DetachRepositoryFromProject` also clear `project.default_repo_id` if the detached repo was the primary?
   - **Recommendation:** Yes, add this check for consistency
