# Implementation Tasks

## Backend API

- [ ] Add `MoveProjectRequest` type in `api/pkg/types/project.go` with `OrganizationID` field
- [ ] Add `moveProject` handler in `api/pkg/server/project_handlers.go`:
  - Validate user owns the project (`project.UserID == user.ID`)
  - Validate user is member of target org (`authorizeOrgMember`)
  - Validate `organization_id` is not empty
  - Update `project.OrganizationID` in a transaction
  - Update `git_repositories.organization_id` for linked repos
  - Update `project_repositories.organization_id` for junction entries
  - Add audit log entry for the move operation
- [ ] Register route `POST /api/v1/projects/{id}/move` in `api/pkg/server/server.go`
- [ ] Add swagger annotations for the new endpoint

## Testing

- [ ] Add unit test for `moveProject` handler:
  - Success case: personal project moved to org
  - Error: user not project owner
  - Error: user not org member
  - Error: project not found
  - Error: empty organization_id

## API Client (Optional)

- [ ] Run `./stack update_openapi` to regenerate TypeScript client
- [ ] Add `MoveProject` method to Go client in `api/pkg/client/` if needed