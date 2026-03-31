# Design

## Root Cause

`submitDesignReview` in `api/pkg/server/spec_task_design_review_handlers.go` (lines 282–325).

The `"approve"` switch case (lines 283–288) only updates the `review` object. It never touches `specTask`. Compare with `"request_changes"` (lines 289–321) which updates task status, saves the task, and pings the agent.

## Fix

In the `"approve"` case, after setting review fields, add the same task transition that already exists in `approveImplementation`'s auto-approve branch (lines 80–96 of `spec_task_workflow_handlers.go`):

```go
case "approve":
    review.Status = types.SpecTaskDesignReviewStatusApproved
    now := time.Now()
    review.ApprovedAt = &now
    review.OverallComment = req.OverallComment

    // Advance task status
    specTask.Status = types.TaskStatusSpecApproved
    specTask.SpecApprovedBy = user.ID
    specTask.SpecApprovedAt = &now
    specTask.StatusUpdatedAt = &now
    if err := s.Store.UpdateSpecTask(ctx, specTask); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    // Kick off implementation asynchronously
    s.wg.Add(1)
    go func() {
        defer s.wg.Done()
        if err := s.specDrivenTaskService.ApproveSpecs(context.Background(), specTask); err != nil {
            log.Error().Err(err).Str("spec_task_id", specTask.ID).Msg("[DesignReview] Failed to start implementation after approval")
        }
    }()
```

## Key Files

| File | Purpose |
|------|---------|
| `api/pkg/server/spec_task_design_review_handlers.go` | The bug — `approve` case missing task update |
| `api/pkg/server/spec_task_workflow_handlers.go` | Reference — `approveImplementation` auto-approve branch (lines 74–100) |
| `api/pkg/services/spec_driven_task_service.go` | `ApproveSpecs()` — triggers implementation |
| `api/pkg/types/simple_spec_task.go` | `TaskStatusSpecApproved` constant |

## Notes

- `SpecApprovedBy`/`SpecApprovedAt` fields may or may not exist on the struct — check `types.SpecTask` before adding them. If they don't exist, just update the status; the `ApproveSpecs()` call is the critical part.
- The `context.Background()` goroutine pattern is already established in `approveImplementation` — follow the same pattern.
- No frontend changes needed if the frontend already handles `spec_approved` status correctly.
