# Fix spec tasks stuck in infinite spec_approved processing loop

## Summary
When a user clicks "Approve Implementation" while a task is still in `spec_review`, the handler's fallback path sets status to `spec_approved` but forgets to populate the `SpecApproval` JSONB field. This causes `ApproveSpecs()` to fail with "spec approval not found", leaving the task permanently stuck while the orchestrator retries every 10 seconds.

## Changes
- **spec_task_workflow_handlers.go**: Populate `specTask.SpecApproval` in the `approveImplementation` fallback path, matching what the normal spec approval handler does
- **spec_driven_task_service.go**: Instead of returning an error when `SpecApproval` is nil, synthesize one from the task's `SpecApprovedBy`/`SpecApprovedAt` fields — this self-heals tasks already stuck in the DB
- **spec_task_orchestrator.go**: Tighten the error filter from `"not found"` to `"record not found"` so that `"spec approval not found"` errors are logged at ERROR level instead of being silently swallowed at TRACE
