# Implementation Tasks

## Backend API

- [x] Add `MoveProjectRequest` type in `api/pkg/types/project.go` with `OrganizationID` field
- [x] Add `moveProject` handler in `api/pkg/server/project_handlers.go`:
  - Validate user owns the project (`project.UserID == user.ID`)
  - Validate user is member of target org (`authorizeOrgMember`)
  - Validate `organization_id` is not empty
  - Update `project.OrganizationID` in a transaction
  - Update `git_repositories.organization_id` for linked repos
  - Update `project_repositories.organization_id` for junction entries
  - Handle naming conflicts: rename project using `(1)`, `(2)` pattern if needed
  - Handle naming conflicts: rename repos using `-2`, `-3` pattern if needed
  - Add audit log entry for the move operation
- [x] Add `moveProjectPreview` handler in `api/pkg/server/project_handlers.go`:
  - Check for project name conflicts in target org
  - Check for repository name conflicts in target org
  - Return proposed renames without making changes
- [x] Register routes in `api/pkg/server/server.go`:
  - `POST /api/v1/projects/{id}/move` - execute move
  - `POST /api/v1/projects/{id}/move/preview` - check conflicts
- [x] Add swagger annotations for both endpoints

## Frontend UI

- [x] Add "Move to Organization" section in Danger Zone of `ProjectSettings.tsx`:
  - Only render when `!project?.organization_id` (personal projects only)
  - Follow existing Danger Zone box styling pattern
- [x] Add organization select dropdown:
  - Use `account.organizationTools.organizations` for options
  - Disable move button until org selected
- [x] Call preview endpoint when org is selected:
  - Show loading state while checking conflicts
  - Display list of actual repository names that will be moved
  - Show any naming conflicts with proposed renames (e.g., "api" → "api-2")
- [x] Add confirmation dialog:
  - Warn that this is a one-way operation
  - Show target organization name
  - List repositories by name (with rename arrows if conflicts exist)
  - Explain that repos will become accessible to org members
  - Require explicit confirmation
- [x] Call API on confirm:
  - Use generated client method after `./stack update_openapi`
  - Invalidate project query on success
  - Show success/error snackbar

## Testing

- [ ] Add unit test for `moveProject` handler (future work):
  - Success case: personal project moved to org
  - Error: user not project owner
  - Error: user not org member
  - Error: project not found
  - Error: empty organization_id

## API Client

- [x] Run `./stack update_openapi` to regenerate TypeScript client
- [x] Add `MoveProject` method to Go client in `api/pkg/client/` if needed (not needed - generated client sufficient)