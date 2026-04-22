# Implementation Tasks

- [ ] In `api/pkg/server/spec_task_workflow_handlers.go` (~line 81): in the `approveImplementation` fallback for `spec_review`/`spec_approved` tasks, set `specTask.SpecApproval = &types.SpecApprovalResponse{Approved: true, ApprovedBy: user.ID, ApprovedAt: now}` before the `UpdateSpecTask` call. Note: `ApprovedAt` is `time.Time` (value, not pointer)
- [ ] In `api/pkg/services/spec_driven_task_service.go` (~line 1139): replace the `return fmt.Errorf("spec approval not found")` with code that synthesizes a `SpecApprovalResponse` from the task's existing `SpecApprovedBy`/`SpecApprovedAt` fields (dereference `*task.SpecApprovedAt` with nil-guard since it's `*time.Time`), allowing already-stuck tasks to self-heal
- [ ] In `api/pkg/services/spec_task_orchestrator.go` (~line 251): tighten the error filter from `strings.Contains(err.Error(), "not found")` to `strings.Contains(err.Error(), "record not found")` so that "spec approval not found" is logged at ERROR level
- [ ] Verify `go build ./...` passes
- [ ] Test: confirm the normal UI spec approval flow still works (SpecApproval set by handler, ApproveSpecs succeeds)
- [ ] Test: confirm clicking "Approve Implementation" on a task in `spec_review` status now correctly sets SpecApproval and transitions to implementation
