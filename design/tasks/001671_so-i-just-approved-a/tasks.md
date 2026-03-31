# Implementation Tasks

- [ ] Check `types.SpecTask` struct for `SpecApprovedBy`/`SpecApprovedAt` fields
- [ ] In `submitDesignReview` `"approve"` case: update `specTask.Status = TaskStatusSpecApproved`, set approval metadata, call `s.Store.UpdateSpecTask()`
- [ ] In `submitDesignReview` `"approve"` case: launch goroutine calling `s.specDrivenTaskService.ApproveSpecs()` (same pattern as `approveImplementation` auto-approve branch)
- [ ] Build and verify: `go build ./pkg/server/ ./pkg/types/`
- [ ] End-to-end test: approve a design review in the inner Helix UI and confirm the task advances to `spec_approved` / implementation
