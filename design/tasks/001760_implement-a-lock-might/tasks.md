# Implementation Tasks

## Backend

- [x] Add `KeepAlive bool` field to `SpecTask` struct in `api/pkg/types/simple_spec_task.go`
- [x] Add `KeepAlive *bool` field to `SpecTaskUpdateRequest` in same file
- [x] Handle `KeepAlive` update in `updateSpecTask` handler in `api/pkg/server/spec_driven_task_handlers.go`
- [x] Clear `KeepAlive` in the backlog-reset block in `updateSpecTask`
- [x] Add NOT EXISTS filter to `ListIdleDesktops` SQL query in `api/pkg/store/store_sessions.go` to skip keep-alive tasks
- [x] Add test case in `api/pkg/store/store_desktop_idle_test.go` — verify keep-alive task is excluded from idle list
- [x] Add swagger annotation for the new field and run `./stack update_openapi`

## Frontend

- [x] Add Keep Alive toggle button to header toolbar in `SpecTaskDetailContent.tsx` (after Stop, before Upload)
- [x] Wire toggle to `updateSpecTask` mutation with `{ keep_alive: !task.keep_alive }` payload
- [x] Verify generated API client includes `keep_alive` field after openapi regen

## Testing

- [x] Test: toggle on → idle checker SQL excludes session (verified via direct DB query)
- [x] Test: toggle off → idle checker SQL includes session again (verified)
- [x] Test: API toggle on/off via PUT /api/v1/spec-tasks/{id} with keep_alive field (verified)
- [x] Test: toggle state persists across API re-fetch (verified)
- [x] Build Go: `go build ./api/...`
- [x] Build frontend: `cd frontend && tsc --noEmit` (dist permissions issue prevents full build, transform succeeds)
- [x] Fix: removed `deleted_at IS NULL` from NOT EXISTS subquery (spec_tasks has no deleted_at column)
