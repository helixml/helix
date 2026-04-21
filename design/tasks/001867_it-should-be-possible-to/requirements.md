# Requirements: Mark Task as Done Without a Pull Request

## User Stories

**As a project user**, I want to mark a task as "done" without going through the full implementation/PR workflow, so that I can close tasks that turned out not to need code changes.

### Use Cases

- Task was investigated and determined to be a non-issue
- The fix was applied manually or through a different process
- The task is a duplicate of work already completed
- The desired behavior already exists — no change needed

## Acceptance Criteria

1. A "Mark as Done" action is available in the task card's three-dot menu for tasks that are not already done or archived
2. Clicking "Mark as Done" shows a confirmation dialog (consistent with the existing archive confirmation pattern)
3. After confirmation, the task transitions to `done` status with `CompletedAt` set
4. The task appears in the "Completed" column on the kanban board
5. A completed-without-PR task can be reopened using the existing "Reopen" action
6. The action is available from any pre-done status (backlog, spec_review, implementation, pull_request, etc.)

## Out of Scope

- Adding a reason/comment field for why the task was closed without PR (can be added later)
- Changing the backend API (the `PUT /api/v1/spec-tasks/{taskId}` endpoint already supports direct status updates)
- Modifying the task status enum or adding a new status
