# Fix spec tasks stuck in infinite spec_approved processing loop

## Summary
The Design Review UI's "Approve Design" action made two sequential API calls: one to mark the review record as approved, and a separate one to approve the spec task. If the second call failed (network blip, tab close), the review showed "approved" but the spec task stayed in `spec_review` with `SpecApproval == nil`. The customer would then see a "Start Implementation" button, click it, and hit a fallback path that set the task to `spec_approved` without populating `SpecApproval`. This caused `ApproveSpecs()` to fail with "spec approval not found" every 10 seconds in the orchestrator, permanently stuck.

## Changes

**Root cause fix:**
- **spec_task_design_review_handlers.go**: Move spec task approval into the `submitDesignReview` handler's "approve" case, making both writes atomic in one HTTP request. Added idempotency guard (early return if review already approved, only transition task from spec-phase statuses).
- **DesignReviewContent.tsx**: Remove the now-redundant second API call (`v1SpecTasksApproveSpecsCreate`) from `handleSubmitReview`

**Eliminate reverse drift:**
- **spec_driven_task_handlers.go**: The `approveSpecs` endpoint (still used by CloneGroupProgress, useApproveSpecTask, etc.) now also syncs the latest DesignReview record to "approved" when one exists, preventing review/task status drift in either direction

**Defense-in-depth:**
- **spec_task_workflow_handlers.go**: Populate `specTask.SpecApproval` (including `TaskID`) in the `approveImplementation` fallback path
- **spec_driven_task_service.go**: Synthesize `SpecApproval` from `SpecApprovedBy`/`SpecApprovedAt` when nil (falls back to `time.Now()` instead of zero time), self-healing tasks already stuck in DB
- **spec_task_orchestrator.go**: Extract `isDeletedProjectError()` function, tighten all 3 error filter sites to only match GORM `"record not found"`, not domain errors like `"spec approval not found"`

**Tests:**
- `TestApproveSpecs_SynthesizesNilSpecApproval` — verifies self-heal with `TaskID` and `ApprovedAt`
- `TestApproveSpecs_NilSpecApprovalAndNilApprovedAt` — worst case, falls back to `time.Now()`
- `TestHandleSpecApproved_SelfHealsNilSpecApproval` — end-to-end through orchestrator
- `TestIsDeletedProjectError` — tests extracted filter function with both matching and non-matching cases
- `TestProcessTask_ErrorFilterDistinguishesNotFoundTypes` — uses extracted function
