# Requirements: Fix Spec Tasks Stuck in Infinite `spec_approved` Loop

## User Stories

### 1. "Approve Implementation" works even when specs haven't been formally approved yet
**As** a user who clicks "Approve Implementation" while a task is still in `spec_review`,
**I want** the system to correctly handle the implicit spec approval (the fallback path inside `approveImplementation`),
**So that** my task advances to implementation instead of getting permanently stuck.

**Context:** The `approveImplementation` handler (`spec_task_workflow_handlers.go:72-101`) has a fallback: if the task is in `spec_review` or `spec_approved` when the user approves implementation, it auto-approves specs first as a prerequisite. This fallback sets `SpecApprovedBy`/`SpecApprovedAt`/status but forgets to set the `SpecApproval` JSONB field, which `ApproveSpecs()` then requires to proceed.

### 2. Orchestrator stops retrying permanently-broken tasks
**As** an operator monitoring the system,
**I want** the orchestrator to detect when a `spec_approved` task cannot make progress and stop retrying,
**So that** stuck tasks don't consume compute indefinitely with no backoff.

### 3. Error is visible in logs
**As** a developer debugging stuck tasks,
**I want** the "spec approval not found" error to be logged at ERROR level (not swallowed by the TRACE-level "deleted project" filter),
**So that** I can detect the problem from logs without manual DB inspection.

## Acceptance Criteria

1. When a user clicks "Approve Implementation" and the task is in `spec_review`, the fallback path in `approveImplementation` populates the `SpecApproval` JSONB field before calling `ApproveSpecs()`.
2. `ApproveSpecs()` succeeds on these tasks — the task transitions from `spec_approved` → `implementation` without error.
3. The orchestrator's error handler distinguishes "spec approval not found" from "deleted project not found" and logs the former at ERROR level.
4. If `handleSpecApproved()` fails repeatedly for the same task, the orchestrator applies a retry limit or backoff (not infinite 10-second polling).
5. Existing normal UI spec approval flow (`spec_driven_task_handlers.go`) continues to work unchanged.
6. No new database tables or migrations required — the fix uses the existing `spec_approval` JSONB column on `spec_tasks`.
