# Implementation Tasks

## Backend

- [ ] Add `PinnedProject` type to `api/pkg/types/project.go` with `ID`, `UserID`, `ProjectID`, `CreatedAt` fields
- [ ] Add `TableName()` method returning `"pinned_projects"` for GORM
- [ ] Add `PinnedProject` to AutoMigrate list in `api/pkg/store/postgres.go`
- [ ] Add store methods: `PinProject(ctx, userID, projectID)`, `UnpinProject(ctx, userID, projectID)`, `GetPinnedProjectIDs(ctx, userID)`
- [ ] Add `pinProject` handler in `api/pkg/server/project_handlers.go` (POST `/projects/{id}/pin`)
- [ ] Add `unpinProject` handler in `api/pkg/server/project_handlers.go` (DELETE `/projects/{id}/pin`)
- [ ] Register routes in `api/pkg/server/server.go`
- [ ] Update `listProjects` to include `is_pinned` field by joining with `pinned_projects` table
- [ ] Add swagger annotations for new endpoints
- [ ] Run `./stack update_openapi` to regenerate API client

## Frontend

- [ ] Add `usePinProject` mutation hook in `frontend/src/services/projectService.ts`
- [ ] Add `useUnpinProject` mutation hook in `frontend/src/services/projectService.ts`
- [ ] Update `ProjectsListView.tsx` to separate pinned/unpinned projects
- [ ] Add "Pinned" section header with pin icon above pinned projects grid
- [ ] Add "Pin" menu item to project card 3-dot menu (for unpinned projects)
- [ ] Add "Unpin" menu item to project card 3-dot menu (for pinned projects)
- [ ] Hide pinned section when no projects are pinned

## Testing

- [ ] Verify pin/unpin works in personal workspace
- [ ] Verify pin/unpin works in organization context
- [ ] Verify pinned state persists after page refresh
- [ ] Verify different users have independent pin states