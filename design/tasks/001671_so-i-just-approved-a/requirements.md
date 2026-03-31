# Requirements

## Problem

When a user approves a design review, the task stays in `spec_review` status because the `approve` handler never updates the task status. The orchestrator's `handleSpecReview` is a no-op (returns nil, waits for human action), so the task sits in `spec_review` indefinitely and the UI stays on the design review screen. Implementation never starts.

The `request_changes` path works correctly: it sets `TaskStatusSpecRevision`, saves the task, and notifies the agent. The `approve` path does none of these things.

## User Stories

**As a user approving a design**, I expect the task to advance to implementation after I click Approve — not stay on the design review screen.

## Acceptance Criteria

- Approving a design review transitions the task status from `spec_review` → `spec_approved`
- The task records who approved it (`SpecApprovedBy`, `SpecApprovedAt`)
- `ApproveSpecs()` is called asynchronously to kick off implementation (same as the auto-approve path in `approveImplementation`)
- The API response still returns the updated design review object (unchanged)
- `request_changes` path is unaffected
