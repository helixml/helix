# Design

## Root cause

Three independent gaps make the task "disappear":

1. `NewSpecTaskForm.tsx` has no assignee field.
2. `types.CreateTaskRequest` (`api/pkg/types/simple_spec_task.go:73-91`) has no `AssigneeID` field.
3. `specDrivenTaskService.CreateTaskFromPrompt` (`api/pkg/services/spec_driven_task_service.go:174-196`) never sets `task.AssigneeID`.

So new tasks always have `assignee_id = ""`. The kanban board filter (`SpecTaskKanbanBoard.tsx:877-891`) treats `""` as `__unassigned__` — only included when the user explicitly selects "Unassigned" in the filter dropdown. Result: a newly-created task with an active assignee filter is excluded from view.

## Approach

Add `assignee_id` end-to-end on the create path, mirroring the existing update path. Default the form to the current user since that matches the most common workflow ("I am creating this task and taking it on"), and it neatly resolves the disappearing-task complaint when the active filter is "show me my tasks".

## Backend changes

### 1. `api/pkg/types/simple_spec_task.go`
Add `AssigneeID` to `CreateTaskRequest`:

```go
AssigneeID string `json:"assignee_id,omitempty"` // Optional: team member assigned to the task
```

Use a plain `string` (not `*string`) — on create there's no "clear vs leave alone" ambiguity, unlike update.

### 2. `api/pkg/services/spec_driven_task_service.go`
In `CreateTaskFromPrompt`, when building the `SpecTask` struct (lines 174-196), copy `req.AssigneeID` into `task.AssigneeID`.

### 3. `api/pkg/server/spec_driven_task_handlers.go`
In `createTaskFromPrompt` (lines 33-97), after authorising the user and before calling the service, validate the assignee if non-empty. Extract the existing validation block from `updateSpecTask` (lines 1053-1075) into a small helper (e.g. `validateAssigneeIsOrgMember(ctx, orgID, assigneeID) error`) and call it from both call sites. This keeps the rule in one place. Return HTTP 400 with the same `"assignee must be an organization member"` message on failure.

The org ID needed for validation comes from the project (the handler already authorises against `req.ProjectID`); fetch the project once and reuse `project.OrganizationID`. If the existing flow already loads the project, reuse that load — don't double-fetch.

### 4. Regenerate API client
After the swagger annotations on `createTaskFromPrompt` are still accurate (the request body type reference covers the new field), run `./stack update_openapi` so the frontend's generated client picks up `assignee_id`.

## Frontend changes

### 5. `frontend/src/components/tasks/NewSpecTaskForm.tsx`
Add an assignee field to the form. Place it near the priority selector (around lines 448-460) so the meta-fields cluster together.

- Add local state `const [assigneeId, setAssigneeId] = useState<string>(currentUserId ?? "")`.
- Reuse the existing `AssigneeSelector` popover component (`AssigneeSelector.tsx`) — it already handles member list, "Unassigned" option, current-user-on-top sorting, and avatars. Trigger it from a button styled to match the priority selector.
- Source `members` and `currentUserId` the same way `TaskCard.tsx` does (lines 564-570): `account.organizationTools.organization?.memberships`. The form already has access to the account context — verify and reuse.
- Include `assignee_id: assigneeId` in the body passed to `api.getApiClient().v1SpecTasksFromPromptCreate(...)` (around line 351).

### 6. `frontend/src/components/tasks/AssigneeSelector.tsx`
No changes expected — its props (`assigneeId`, `members`, `currentUserId`, `onAssigneeChange`, `anchorEl`, `onClose`) already cover this use case. If it is currently coupled to a `TaskCard`-specific layout (e.g. positioning), leave the component alone and add a small wrapper button in `NewSpecTaskForm` to anchor the popover.

## Why default to "current user"?

Two reasons:
- Matches the common case — most tasks are created by the person who plans to do them.
- Directly fixes the reported symptom: if the user filters to "my tasks", their newly created task lands in the visible set without surprise.

Users who want to delegate can change the field before submitting; users who want it unassigned can pick "Unassigned" explicitly.

## Validation parity

The rule today (in update): assignee must be an organization member. We apply the same rule to create. We do NOT add new rules (e.g. "must be a project member") — that would be a wider scope change and isn't part of the bug.

## Test strategy

- **Go unit test**: extend the existing `spec_driven_task_handlers` test suite (or add a focused test) that POSTs to `/api/v1/spec-tasks/from-prompt` with (a) no `assignee_id` → unassigned task, (b) valid org-member `assignee_id` → assigned task, (c) non-member `assignee_id` → HTTP 400.
- **Frontend**: manual browser test in the inner Helix per `CLAUDE.md` — register/log in, create a task, confirm assignee defaults to current user, change to another member, change to Unassigned, submit and confirm the resulting task shows the correct assignee on its card.
- **Bug reproduction**: with assignee filter set to "current user only", create a new task and confirm it appears immediately on the board (no longer disappears).

## Notes for future agents

- The kanban assignee filter is **client-side only** (`SpecTaskKanbanBoard.tsx:877-891`). `GET /api/v1/spec-tasks` does not accept `assignee_id`. Don't be tempted to add backend assignee filtering as part of this task — it's out of scope and the data volume doesn't need it.
- `AssigneeSelector` is reusable across the app — it does no data fetching, just receives `members` as a prop. Reuse it rather than building a new selector.
- The update endpoint uses `*string` for `AssigneeID` to distinguish "clear" from "no change". Create doesn't need the pointer — empty string already means "no assignee".
