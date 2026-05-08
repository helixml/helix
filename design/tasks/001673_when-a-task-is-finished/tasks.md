# Implementation Tasks

- [x] Add `DismissAttentionEventsForTask(ctx, specTaskID) (int64, error)` to the `Store` interface in `api/pkg/store/store.go` (next to the other Attention Event methods around line 598-604)
- [x] Implement `DismissAttentionEventsForTask` in `api/pkg/store/store_attention_events.go` (mirror `BulkDismissAttentionEvents`, scoped by `spec_task_id` with `dismissed_at IS NULL` guard)
- [x] Regenerate / update `api/pkg/store/store_mocks.go` so gomock has the new method
- [x] Add a small helper (e.g. `dismissTaskNotifications(ctx, store, taskID)`) somewhere shared by `git_http_server.go` and `spec_task_orchestrator.go` — log-and-swallow errors, don't propagate
- [x] Call the helper after every Done transition in `git_http_server.go` (2 sites), `spec_task_orchestrator.go` (4 sites), and `spec_task_workflow_handlers.go` (1 site — discovered during impl: the user-driven Approve Implementation server-side merge path)
- [x] Add a unit test in `api/pkg/store/store_attention_events_test.go` covering: dismisses only the target task's active events, leaves other tasks' events alone, idempotent, unknown task ID is no-op, empty task ID is a validation error. (Compiles and passes `go vet`. Per CLAUDE.md store tests need Postgres — runs in CI.)
- [ ] **WARNING: NOT manually tested in inner Helix yet** — no local Postgres / inner Helix instance available in this sandbox (`localhost:8080` returns 000). Functional verification will happen via CI + reviewer's manual smoke test.
- [x] Build green (`go build ./api/...` passes after each change)
- [x] Update design.md "Notes for future agents" with the seventh call site discovered (workflow handler)
- [x] Write `pull_request_helix.md`
