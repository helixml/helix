# Requirements: Fix Spec Tasks Stuck in Infinite `spec_approved` Loop

## User Stories

### 1. Auto-approval creates complete state
**As** the controlplane operator,
**I want** the auto-approval path to create all required records (including `SpecApproval`) before transitioning to `spec_approved`,
**So that** `ApproveSpecs()` can proceed without failing on a missing record.

### 2. Orchestrator stops retrying permanently-broken tasks
**As** an operator monitoring the system,
**I want** the orchestrator to detect when a `spec_approved` task cannot make progress and stop retrying,
**So that** stuck tasks don't consume compute indefinitely with no backoff.

### 3. Error is visible in logs
**As** a developer debugging stuck tasks,
**I want** the "spec approval not found" error to be logged at ERROR level (not swallowed by the TRACE-level "deleted project" filter),
**So that** I can detect the problem from logs without manual DB inspection.

## Acceptance Criteria

1. When the auto-approval path in `spec_task_workflow_handlers.go` fires, the task's `SpecApproval` field (the `*SpecApprovalResponse` JSONB column) is populated **before** `ApproveSpecs()` is called.
2. `ApproveSpecs()` succeeds on auto-approved tasks — the task transitions from `spec_approved` → `implementation` without error.
3. The orchestrator's error handler distinguishes "spec approval not found" from "deleted project not found" and logs the former at ERROR level.
4. If `handleSpecApproved()` fails repeatedly for the same task, the orchestrator applies a retry limit or backoff (not infinite 10-second polling).
5. Existing normal UI approval flow (`spec_driven_task_handlers.go`) continues to work unchanged.
6. No new database tables or migrations required — the fix uses the existing `spec_approval` JSONB column on `spec_tasks`.
