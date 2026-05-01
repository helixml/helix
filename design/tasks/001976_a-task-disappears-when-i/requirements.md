# Requirements

## Problem

A user creates a new task while an assignee filter is active on the Kanban board. The task is created with no assignee (`assignee_id = ""`), so the kanban board's client-side filter excludes it from the visible columns. The user believes the task creation failed because it "disappeared". Today the only way to assign a task is via the `AssigneeSelector` popover on an already-created `TaskCard` — there is no way to set an assignee at creation time.

## User Stories

### Story 1: Set assignee at creation time
**As a** project member creating a new task,
**I want to** choose who the task is assigned to as part of the create-task form,
**so that** I don't have to do a second click to assign it after creation.

**Acceptance criteria:**
- The "New Task" form (`NewSpecTaskForm`) includes an assignee field that lists organization members.
- The field defaults to the current user (the most common case: "I'm taking this on").
- The user can change the default to any other org member, or explicitly choose "Unassigned".
- The selected assignee is sent to the backend and stored on the new task.

### Story 2: New task is visible after creation when a filter is set
**As a** project member with an assignee filter active,
**I want** the task I just created to appear in the kanban board,
**so that** I can see and continue working with it.

**Acceptance criteria:**
- When the form defaults the assignee to the current user, and the current user is in the active assignee filter, the new task appears immediately on the board.
- If the user explicitly chooses an assignee that is *not* in the active filter, no special handling is required — the task is created correctly; the filter behaviour is the user's choice. (Out of scope: warning the user that their new task won't match the current filter.)

### Story 3: Backend accepts and validates assignee on create
**As a** developer of the API,
**I want** the create-task endpoint to accept an optional `assignee_id` and validate it,
**so that** invalid assignees (non-members) are rejected at the boundary, matching the existing update-task behaviour.

**Acceptance criteria:**
- `POST /api/v1/spec-tasks/from-prompt` accepts an optional `assignee_id` field.
- If supplied, the backend validates that the assignee is a member of the task's organization (mirroring the validation in `updateSpecTask` at `api/pkg/server/spec_driven_task_handlers.go:1053-1075`).
- Invalid assignee → HTTP 400 with the existing error message `"assignee must be an organization member"`.
- If omitted or empty, the task is created unassigned (current behaviour preserved for any callers that don't supply it, e.g. CLI / API consumers).

## Out of Scope

- Backend filtering by `assignee_id` on `GET /api/v1/spec-tasks` (the board still filters client-side; that's fine for the data volumes involved).
- Bulk re-assignment.
- Warning the user when their chosen assignee won't match an active filter.
- Changing how the kanban filter treats unassigned tasks.
