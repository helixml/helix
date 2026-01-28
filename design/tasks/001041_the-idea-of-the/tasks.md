# Implementation Tasks

## Backend

- [ ] Add `PublicDesignDocs bool` field to `SpecTask` struct in `api/pkg/types/simple_spec_task.go`
- [ ] Add `PublicDesignDocs *bool` to `SpecTaskUpdateRequest` in `api/pkg/types/simple_spec_task.go`
- [ ] Update `viewDesignDocsPublic` handler in `api/pkg/server/spec_task_share_handlers.go` to check `PublicDesignDocs` before requiring token
- [ ] Create user-friendly "This spec task is private" HTML template for unauthorized access
- [ ] Update spec task PATCH handler to allow setting `public_design_docs` field
- [ ] Run `./stack update_openapi` to regenerate API client

## Frontend

- [ ] Add public toggle switch to `SpecTaskReviewPanel.tsx` above the share link section
- [ ] Add mutation to update `public_design_docs` via PATCH endpoint
- [ ] Show "Copy Public Link" button when public is enabled (simple URL without token)
- [ ] Rename existing button to "Get Temporary Link" to distinguish from public link
- [ ] Update UI to show current public/private state from task data

## Testing

- [ ] Test public view works without token when `PublicDesignDocs` is true
- [ ] Test public view requires token when `PublicDesignDocs` is false
- [ ] Test only task owner can toggle public access
- [ ] Test admin can toggle public access on any task