# Requirements

## Problem

When a user approves a design review, the task stays in `spec_review` status because the `approve` handler branch never updates the task status. The agent (still alive, watching for status changes or re-triggered by the orchestrator) interprets `spec_review` as "keep generating specs" and restarts spec writing.

The `request_changes` path works correctly: it sets `TaskStatusSpecRevision`, saves the task, and notifies the agent. The `approve` path does none of these things.

## User Stories

**As a user approving a design**, I expect the task to advance to implementation after I click Approve — not loop back to spec generation.

## Acceptance Criteria

- Approving a design review transitions the task status from `spec_review` → `spec_approved`
- The task records who approved it (`SpecApprovedBy`, `SpecApprovedAt`)
- `ApproveSpecs()` is called asynchronously to kick off implementation (same as the auto-approve path in `approveImplementation`)
- The API response still returns the updated design review object (unchanged)
- `request_changes` path is unaffected
