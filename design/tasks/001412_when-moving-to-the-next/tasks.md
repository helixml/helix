# Implementation Tasks

- [x] Add `StatusUpdatedAt *time.Time` field to `SpecTask` struct in `api/pkg/types/simple_spec_task.go`
- [x] Run `./stack update_openapi` to regenerate API client with new field
- [~] Update `updateSpecTask` handler in `api/pkg/server/spec_driven_task_handlers.go` to set `StatusUpdatedAt = time.Now()` when `updateReq.Status != ""`
- [ ] Update `createSpecTask` handler to set `StatusUpdatedAt = CreatedAt` for new tasks
- [ ] Change sort order in `ListSpecTasks` in `api/pkg/store/store_spec_tasks.go` from `created_at DESC` to `status_updated_at DESC NULLS LAST, created_at DESC`
- [ ] Test: Create new task, verify it appears at top of backlog
- [ ] Test: Move task to different column, verify it appears at top of new column
- [ ] Test: Verify existing tasks without `status_updated_at` still appear (sorted by `created_at`)