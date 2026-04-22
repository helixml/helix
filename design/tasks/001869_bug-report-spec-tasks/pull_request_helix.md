# Fix spec tasks stuck in infinite spec_approved processing loop

## Summary
The Design Review UI's "Approve Design" action made two sequential API calls: one to mark the review record as approved, and a separate one to approve the spec task. If the second call failed (network blip, tab close), the review showed "approved" but the spec task stayed in `spec_review` with `SpecApproval == nil`. The customer would then see a "Start Implementation" button, click it, and hit a fallback path that set the task to `spec_approved` without populating `SpecApproval`. This caused `ApproveSpecs()` to fail with "spec approval not found" every 10 seconds in the orchestrator, permanently stuck.

## Changes
- **spec_task_design_review_handlers.go**: Move spec task approval into the `submitDesignReview` handler's "approve" case, making both writes atomic in one HTTP request (root cause fix)
- **DesignReviewContent.tsx**: Remove the now-redundant second API call (`v1SpecTasksApproveSpecsCreate`) from `handleSubmitReview`
- **spec_task_workflow_handlers.go**: Populate `specTask.SpecApproval` in the `approveImplementation` fallback path (defense-in-depth)
- **spec_driven_task_service.go**: Synthesize `SpecApproval` from `SpecApprovedBy`/`SpecApprovedAt` when nil instead of returning an error (self-heals tasks already stuck in DB)
- **spec_task_orchestrator.go**: Tighten error filter from `"not found"` to `"record not found"` so domain errors aren't silently swallowed at TRACE level
- Added 4 unit tests covering the self-heal path and error filter behavior
