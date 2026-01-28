# Implementation Tasks

## Backend

- [x] Add `PublicDesignDocs bool` field to `SpecTask` struct in `api/pkg/types/simple_spec_task.go`
- [x] Add `PublicDesignDocs *bool` to `SpecTaskUpdateRequest` in `api/pkg/types/simple_spec_task.go`
- [x] Update `viewDesignDocsPublic` handler in `api/pkg/server/spec_task_share_handlers.go` to check `PublicDesignDocs` instead of requiring token
- [x] Create user-friendly "This spec task is private" HTML template for non-public tasks
- [x] Update spec task PATCH handler to allow setting `public_design_docs` field
- [x] Remove `generateDesignDocsShareLink` handler and related types (`DesignDocsShareTokenClaims`, `DesignDocsShareLinkResponse`)
- [x] Remove POST `/api/v1/spec-tasks/{id}/design-docs/share` route from `server.go`
- [x] Run `./stack update_openapi` to regenerate API client

## Frontend

- [x] Replace "Get Shareable Link" section in `SpecTaskReviewPanel.tsx` with public toggle
- [x] Add mutation to update `public_design_docs` via PATCH endpoint
- [x] Show "Copy Link" button when public is enabled (simple URL: `{baseURL}/spec-tasks/{id}/view`)
- [x] Remove token-based share link generation code
- [x] Add public toggle to `SpecTaskDetailContent.tsx` (main task details view)

## Testing

- [ ] Test public view works without login when `PublicDesignDocs` is true
- [ ] Test public view shows "private" message when `PublicDesignDocs` is false
- [ ] Test only task owner can toggle public access
- [ ] Test admin can toggle public access on any task

## Implementation Notes

- Added public toggle in two places:
  1. `SpecTaskReviewPanel.tsx` - standalone review panel component
  2. `SpecTaskDetailContent.tsx` - main task details view (in the "Share Design Docs" section above Archive button)
- Both use the same pattern: Switch toggle + "Copy Public Link" button when enabled
- Uses existing PUT `/api/v1/spec-tasks/{taskId}` endpoint with `public_design_docs` field