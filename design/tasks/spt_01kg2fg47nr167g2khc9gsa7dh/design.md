# Design: Pinned Projects

## Architecture

### Database Model

Create a new `PinnedProject` table to track per-user project pins:

```go
// PinnedProject tracks which projects a user has pinned
type PinnedProject struct {
    ID        string    `json:"id" gorm:"primaryKey"`
    UserID    string    `json:"user_id" gorm:"index:idx_pinned_user_project,unique"`
    ProjectID string    `json:"project_id" gorm:"index:idx_pinned_user_project,unique"`
    CreatedAt time.Time `json:"created_at"`
}
```

**Key decisions:**
- Separate table (not a column on Project) because pin state is per-user, not per-project
- Composite unique index on `(user_id, project_id)` prevents duplicate pins
- No `organization_id` needed - user ID is sufficient since the same user might pin projects across different orgs

### API Endpoints

Add two new endpoints to `project_handlers.go`:

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/projects/{id}/pin` | Pin a project for the current user |
| DELETE | `/api/v1/projects/{id}/pin` | Unpin a project for the current user |

The existing `GET /api/v1/projects` endpoint will be enhanced to return `is_pinned: boolean` for each project.

### Frontend Changes

**ProjectsListView.tsx:**
- Split projects into two arrays: `pinnedProjects` and `unpinnedProjects`
- Render pinned section with header "Pinned" above the regular grid
- Add pin/unpin menu items to project card 3-dot menu

**projectService.ts:**
- Add `usePinProject` and `useUnpinProject` mutation hooks
- Invalidate projects list query on pin/unpin success

## Data Flow

1. User clicks "Pin" on project card menu
2. Frontend calls `POST /api/v1/projects/{id}/pin`
3. Backend creates `PinnedProject` record
4. Frontend invalidates projects query
5. Projects list refetches with `is_pinned: true` for that project
6. Frontend renders project in pinned section

## Patterns Found in Codebase

- **Store pattern**: GORM AutoMigrate handles schema (no SQL migrations) - see `postgres.go`
- **Handler pattern**: Return `(data, *system.HTTPError)` - see existing project handlers
- **Frontend mutations**: Use React Query with `invalidateQueries` on success - see `projectService.ts`
- **Menu items**: Use MUI `<Menu>` and `<MenuItem>` with icon + text - see `ProjectsListView.tsx`
