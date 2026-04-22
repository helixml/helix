# Design: Fix Spec Tasks Stuck in Infinite `spec_approved` Loop

## Root Cause

Two bugs combine to create a permanently stuck state.

### What triggers the bug

The user clicks **"Approve Implementation"** in the UI while the task is still in `spec_review` status (specs were generated but not yet formally approved via the separate "Approve Specs" button). The `approveImplementation` handler (`spec_task_workflow_handlers.go:72-101`) has a fallback for this scenario: it detects the task isn't in `implementation` yet and tries to approve specs as a prerequisite before proceeding.

The bug report attributes this to "the prompt queue's auto-unstick mechanism," but the prompt queue (`prompt_history_handlers.go:79`) just processes pending prompts in the background — it doesn't trigger spec approval. The `approveImplementation` HTTP handler (called by the user's UI action) is the actual trigger.

### Bug 1: `approveImplementation` fallback skips `SpecApproval` field

The **normal "Approve Specs" UI** flow (`spec_driven_task_handlers.go:391`) decodes a `SpecApprovalResponse` from the request body and sets `existingTask.SpecApproval = &req` before saving. The **fallback path inside `approveImplementation`** (`spec_task_workflow_handlers.go:81-89`) sets `SpecApprovedBy`, `SpecApprovedAt`, and `Status` but **never sets `SpecApproval`**. When `ApproveSpecs()` re-reads the task from DB, it finds `task.SpecApproval == nil` at line 1139 of `spec_driven_task_service.go` and returns `"spec approval not found"`.

### Bug 2: Error is swallowed at TRACE level

The orchestrator's error handler (`spec_task_orchestrator.go:251`) checks `strings.Contains(err.Error(), "not found")` to suppress noise from deleted-project references. The string `"spec approval not found"` matches this filter, so the error is logged at TRACE level instead of ERROR. This makes the infinite loop invisible in normal log output.

## Fix

### Fix 1: Populate `SpecApproval` in the `approveImplementation` fallback

In `spec_task_workflow_handlers.go`, around line 81, add:

```go
specTask.SpecApproval = &types.SpecApprovalResponse{
    Approved:   true,
    ApprovedBy: user.ID,
    ApprovedAt: now,
}
```

Note: `SpecApprovalResponse.ApprovedAt` is `time.Time` (value type, not pointer — see `types/simple_spec_task.go:339`), so pass `now` directly.

This mirrors what the normal spec approval handler does at `spec_driven_task_handlers.go:391`. The `SpecApproval` field is a JSONB column on the existing `spec_tasks` table — no migration needed.

### Fix 2: Tighten the "not found" error filter in the orchestrator

In `spec_task_orchestrator.go:251`, change the condition to only match "record not found" (GORM's standard error) rather than any error containing "not found":

```go
if strings.Contains(err.Error(), "record not found") {
```

This ensures `"spec approval not found"` is logged at ERROR, while deleted-project GORM errors still go to TRACE.

### Fix 3 (defensive): Make `ApproveSpecs` resilient to missing `SpecApproval`

In `spec_driven_task_service.go:1139`, instead of returning an error when `SpecApproval` is nil, synthesize one:

```go
if task.SpecApproval == nil {
    approvedAt := time.Time{}
    if task.SpecApprovedAt != nil {
        approvedAt = *task.SpecApprovedAt
    }
    task.SpecApproval = &types.SpecApprovalResponse{
        Approved:   true,
        ApprovedBy: task.SpecApprovedBy,
        ApprovedAt: approvedAt,
    }
}
```

Note: `task.SpecApprovedAt` is `*time.Time` (pointer on the task struct) but `SpecApprovalResponse.ApprovedAt` is `time.Time` (value), so we dereference with a nil-guard. This allows recovery from the inconsistent state even for tasks already stuck in the DB.

## Key Decisions

**Why not add retry limits/backoff to the orchestrator?** The root cause is a missing field, not a transient failure. Fix 1 prevents the bug from occurring; Fix 3 recovers tasks already in the broken state. Adding retry limits would be over-engineering — if the underlying operation fails permanently, the task should either self-heal (Fix 3) or be flagged clearly (Fix 2). No new state machine states or failure modes needed.

**Why not create a separate `spec_approvals` table?** The bug report mentions a `spec_approvals` table, but code inspection shows there is none — approval data lives in the `SpecApproval` JSONB field on `spec_tasks`. The fix stays within the existing schema.

## Files Changed

| File | Change |
|------|--------|
| `api/pkg/server/spec_task_workflow_handlers.go` | Set `SpecApproval` field in `approveImplementation` fallback (~line 81) |
| `api/pkg/services/spec_driven_task_service.go` | Synthesize `SpecApproval` when nil instead of returning error (~line 1139) |
| `api/pkg/services/spec_task_orchestrator.go` | Tighten "not found" filter to "record not found" (~line 251) |
