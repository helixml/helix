# Implementation Tasks

## Backend

- [x] Add `PublicDesignDocs bool` field to `SpecTask` struct in `api/pkg/types/simple_spec_task.go`
- [x] Add `PublicDesignDocs *bool` to `SpecTaskUpdateRequest` in `api/pkg/types/simple_spec_task.go`
- [~] Update `viewDesignDocsPublic` handler in `api/pkg/server/spec_task_share_handlers.go` to check `PublicDesignDocs` instead of requiring token
- [ ] Create user-friendly "This spec task is private" HTML template for non-public tasks
- [ ] Update spec task PATCH handler to allow setting `public_design_docs` field
- [ ] Remove `generateDesignDocsShareLink` handler and related types (`DesignDocsShareTokenClaims`, `DesignDocsShareLinkResponse`)
- [ ] Remove POST `/api/v1/spec-tasks/{id}/design-docs/share` route from `server.go`
- [ ] Run `./stack update_openapi` to regenerate API client

## Frontend

- [ ] Replace "Get Shareable Link" section in `SpecTaskReviewPanel.tsx` with public toggle
- [ ] Add mutation to update `public_design_docs` via PATCH endpoint
- [ ] Show "Copy Link" button when public is enabled (simple URL: `{baseURL}/spec-tasks/{id}/view`)
- [ ] Remove token-based share link generation code

## Testing

- [ ] Test public view works without login when `PublicDesignDocs` is true
- [ ] Test public view shows "private" message when `PublicDesignDocs` is false
- [ ] Test only task owner can toggle public access
- [ ] Test admin can toggle public access on any task