# Implementation Tasks

- [~] Add `DismissAttentionEventsForTask(ctx, specTaskID) (int64, error)` to the `Store` interface in `api/pkg/store/store.go` (next to the other Attention Event methods around line 598-604)
- [ ] Implement `DismissAttentionEventsForTask` in `api/pkg/store/store_attention_events.go` (mirror `BulkDismissAttentionEvents`, scoped by `spec_task_id` with `dismissed_at IS NULL` guard)
- [ ] Regenerate / update `api/pkg/store/store_mocks.go` so gomock has the new method
- [ ] Add a small helper (e.g. `dismissTaskNotifications(ctx, store, taskID)`) somewhere shared by `git_http_server.go` and `spec_task_orchestrator.go` — log-and-swallow errors, don't propagate
- [ ] Call the helper after the Done transition in `api/pkg/services/git_http_server.go:~1046` (right after `s.store.UpdateSpecTask` succeeds)
- [ ] Call the helper after the Done transition in `api/pkg/services/spec_task_orchestrator.go:~799` (all-PRs-merged path)
- [ ] Call the helper after the Done transition in `api/pkg/services/spec_task_orchestrator.go:~856` (direct branch-merge fallback)
- [ ] Call the helper after the Done transitions at `api/pkg/services/spec_task_orchestrator.go:~1080` and `:~1124`
- [ ] Add a unit test in `api/pkg/store/` covering: dismisses only the target task's active events, leaves other tasks' events alone, idempotent (re-run returns 0 rows updated, no error), no rows for an unknown task ID returns 0 with no error
- [ ] Manually verify in the inner Helix (`http://localhost:8080`): create a task, let it accumulate events, merge it, confirm the red dot disappears from its Kanban card and the bell badge count drops within ~10s
- [ ] `cd api && go build ./pkg/store/ ./pkg/services/` to confirm the build is green before pushing
