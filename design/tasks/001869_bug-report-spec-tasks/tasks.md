# Implementation Tasks

- [x] In `api/pkg/server/spec_task_workflow_handlers.go` (~line 81): in the `approveImplementation` fallback for `spec_review`/`spec_approved` tasks, set `specTask.SpecApproval = &types.SpecApprovalResponse{Approved: true, ApprovedBy: user.ID, ApprovedAt: now}` before the `UpdateSpecTask` call. Note: `ApprovedAt` is `time.Time` (value, not pointer)
- [x] In `api/pkg/services/spec_driven_task_service.go` (~line 1139): replace the `return fmt.Errorf("spec approval not found")` with code that synthesizes a `SpecApprovalResponse` from the task's existing `SpecApprovedBy`/`SpecApprovedAt` fields (dereference `*task.SpecApprovedAt` with nil-guard since it's `*time.Time`), allowing already-stuck tasks to self-heal
- [x] In `api/pkg/services/spec_task_orchestrator.go` (~line 251): tighten the error filter from `strings.Contains(err.Error(), "not found")` to `strings.Contains(err.Error(), "record not found")` so that "spec approval not found" is logged at ERROR level
- [x] Verify `go build ./...` passes
- [x] Add unit test: `ApproveSpecs` synthesizes `SpecApproval` when nil (in `spec_driven_task_service_test.go`)
- [x] Add unit test: orchestrator error filter and `handleSpecApproved` self-heal (in `spec_task_orchestrator_test.go`)
- [x] Verify tests pass (4/4 pass)
