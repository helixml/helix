# Implementation Tasks

- [x] Add `StatusUpdatedAt *time.Time` field to `SpecTask` struct in `api/pkg/types/simple_spec_task.go`
- [x] Run `./stack update_openapi` to regenerate API client with new field
- [x] Update `updateSpecTask` handler in `api/pkg/server/spec_driven_task_handlers.go` to set `StatusUpdatedAt = time.Now()` when `updateReq.Status != ""`
- [x] Update `createSpecTask` handler to set `StatusUpdatedAt = CreatedAt` for new tasks
- [x] Change sort order in `ListSpecTasks` in `api/pkg/store/store_spec_tasks.go` from `created_at DESC` to `status_updated_at DESC NULLS LAST, created_at DESC`
- [ ] Test: Create new task, verify it appears at top of backlog
- [ ] Test: Move task to different column, verify it appears at top of new column
- [ ] Test: Verify existing tasks without `status_updated_at` still appear (sorted by `created_at`)

## Implementation Notes

- Updated all places where `task.Status` is set to also set `task.StatusUpdatedAt = &now`:
  - `spec_driven_task_handlers.go`: updateSpecTask, approveSpecs, startPlanning
  - `spec_task_workflow_handlers.go`: approveImplementation (3 paths)
  - `spec_task_design_review_handlers.go`: submitDesignReview (revision path)
  - `git_http_server.go`: handleMainBranchPush, processDesignDocsForBranch
  - `spec_driven_task_service.go`: StartSpecGeneration, StartJustDoItMode, HandleSpecGenerationComplete, ApproveSpecs (2 paths), markTaskFailed, detectAndLinkExistingPR
  - `spec_task_orchestrator.go`: handleBacklog, handleSpecGeneration, handleSpecRevision, handleImplementationQueued, processExternalPullRequestStatus
- Task 4 implemented in store layer: `CreateSpecTask` in `store_spec_tasks.go` sets `StatusUpdatedAt` to current time if not already set
- Sort order changed to `status_updated_at DESC NULLS LAST, created_at DESC` - NULLS LAST ensures existing tasks without the field still appear