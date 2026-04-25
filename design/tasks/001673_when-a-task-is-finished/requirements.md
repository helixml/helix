# Requirements: Auto-clear notifications when a task is finished

## Problem

A spec task accumulates **attention events** (the red-dot notifications surfaced on Kanban cards and in the bell/notifications panel) over its lifetime — `specs_pushed`, `agent_interaction_completed`, `pr_ready`, etc. Once the task is fully merged to main, these notifications still sit in the panel and on the card asking for attention they no longer need.

Today, the user has to either click each card or hit "Dismiss All" globally. The task's own status change to **Done** carries enough information for the system to clear the task's notifications automatically.

## User Stories

### 1. Finished task → notifications gone
As a user, when one of my tasks transitions to **Done** (merged to default branch), I want all of its outstanding attention events to be dismissed automatically, so my notifications panel and Kanban red dots only reflect tasks that still need me.

**Acceptance Criteria:**
- When a `SpecTask` transitions to `TaskStatusDone` and `MergedToMain = true`, every active (`dismissed_at IS NULL`) `AttentionEvent` with that `spec_task_id` is marked `dismissed_at = now()` for all users.
- Cleanup happens at every code path that performs the Done transition:
  - `api/pkg/services/git_http_server.go` (branch-merge detection during push)
  - `api/pkg/services/spec_task_orchestrator.go` (all-PRs-merged + direct branch-merge poll paths)
- The dismissal is **best-effort** (logged but non-fatal). A failure to clear notifications must not roll back the Done transition.
- Idempotent: re-running on an already-Done task is a no-op (no errors, no resurrected events).

### 2. UI reflects the cleared state on next refresh
As a user, I shouldn't see a red dot on a Done card after the next 10s poll cycle.

**Acceptance Criteria:**
- The bell badge count drops by the number of events the task had.
- The TaskCard red dot disappears for that task.
- No frontend changes are required beyond the existing `useAttentionEvents` 10s poll — the dismissal is server-side.

## Out of Scope

- **Archived tasks**: archiving is a separate user action; we already let users dismiss manually. Not changed here. (Could be a follow-up.)
- **Failed tasks** (`spec_failed`, `implementation_failed`): the failure events themselves are the signal the user needs, so we leave them.
- **Browser/OS push notifications** that have already popped: these are owned by the OS and dedup'd by tag; we don't try to recall them.
- **Slack thread replies**: already-posted Slack messages are not retracted.
