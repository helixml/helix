# Design

## Root cause

Three independent gaps make the task "disappear":

1. `NewSpecTaskForm.tsx` has no assignee field.
2. `types.CreateTaskRequest` (`api/pkg/types/simple_spec_task.go:73-91`) has no `AssigneeID` field.
3. `specDrivenTaskService.CreateTaskFromPrompt` (`api/pkg/services/spec_driven_task_service.go:174-196`) never sets `task.AssigneeID`.

So new tasks always have `assignee_id = ""`. The kanban board filter (`SpecTaskKanbanBoard.tsx:877-891`) treats `""` as `__unassigned__` — only included when the user explicitly selects "Unassigned" in the filter dropdown. Result: a newly-created task with an active assignee filter is excluded from view.

## Approach

Add `assignee_id` end-to-end on the create path, mirroring the existing update path. The field is **optional** — both backend and frontend treat empty / "Unassigned" as a valid, first-class state (some users intentionally file tickets with no assignee for later triage). For convenience, the form pre-fills the current user, but clearing it to "Unassigned" is one interaction and submitting unassigned is a normal, supported flow. The default neatly resolves the disappearing-task complaint when the active filter is "show me my tasks", without forcing the field on users who don't want it.

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

## Why default to "current user" (and why the field stays optional)

The field is optional — submitting unassigned is a supported, valid outcome. We pre-fill the current user as a convenience default, for two reasons:

- It matches the common case — most tasks are created by the person who plans to do them.
- It directly fixes the reported symptom: if the user filters to "my tasks", their newly created task lands in the visible set without surprise.

Users who want to delegate can change the field before submitting. Users who intentionally want the task unassigned (e.g. PMs filing tickets for later triage, backlog grooming) pick "Unassigned" explicitly — the option is a peer of the member entries, not an afterthought.

## Validation parity

The rule today (in update): assignee must be an organization member. We apply the same rule to create. We do NOT add new rules (e.g. "must be a project member") — that would be a wider scope change and isn't part of the bug.

## Test strategy

- **Go unit test**: extend the existing `spec_driven_task_handlers` test suite (or add a focused test) that POSTs to `/api/v1/spec-tasks/from-prompt` with (a) `assignee_id` omitted → unassigned task, (b) `assignee_id` empty string → unassigned task (parity with omit), (c) valid org-member `assignee_id` → assigned task, (d) non-member `assignee_id` → HTTP 400.
- **Frontend**: manual browser test in the inner Helix per `CLAUDE.md` — register/log in, create a task, confirm assignee defaults to current user, change to another member, change to Unassigned and submit (verify the task is created unassigned), submit again with default (verify the task is created assigned to current user).
- **Bug reproduction**: with assignee filter set to "current user only", create a new task and confirm it appears immediately on the board (no longer disappears).

## Notes for future agents

- The kanban assignee filter is **client-side only** (`SpecTaskKanbanBoard.tsx:877-891`). `GET /api/v1/spec-tasks` does not accept `assignee_id`. Don't be tempted to add backend assignee filtering as part of this task — it's out of scope and the data volume doesn't need it.
- `AssigneeSelector` is reusable across the app — it does no data fetching, just receives `members` as a prop. Reuse it rather than building a new selector.
- The update endpoint uses `*string` for `AssigneeID` to distinguish "clear" from "no change". Create doesn't need the pointer — empty string already means "no assignee".

## Implementation notes (post-build)

- **Default-fill timing**: `account.user?.id` may be `undefined` at first render. The form uses an `assigneeTouched` flag set whenever the user opens the popover — once the user has made any choice (including explicitly picking "Unassigned"), the effect that pre-fills `currentUserId` no longer runs. Without this flag, an explicit "Unassigned" choice would silently get overwritten when the account loaded a beat later.
- **Re-fetching the project**: The handler loads the project a second time (the first load is inside `authorizeUserToProjectByID`). The original design preferred avoiding the double-fetch, but threading the project through the authz helper or moving validation into the service was a wider refactor than this bug warranted. The cost is one keyed lookup per task creation — negligible. Keeping validation at the handler boundary keeps the create and update paths symmetric and lets them share the same `validateAssigneeIsOrgMember` helper.
- **Helper signature**: `validateAssigneeIsOrgMember(ctx, orgID, assigneeID)` returns nil when `assigneeID == ""` so callers don't need to gate on empty themselves. Logging stays at the call site so each path can attach its own context (`task_id` for update, `project_id` for create).
- **Pre-existing build break on main**: The base commit when this work started (`082e46abd`) had a stale `m.runnerController` reference in `api/pkg/openai/manager/provider_manager.go` that prevented `go test ./pkg/server/...` from compiling. Origin/main had already fixed it (commits up to `2bd12a1d0`); rebasing onto current main was required to run the new tests locally. Generated swagger/TS files conflicted on the rebase — taking the rebase target's version and re-running `./stack update_openapi` was the cleanest resolution.
- **Manual UI verification deferred**: This environment doesn't have an inner Helix instance running, so the browser-level repro of the original "task disappears" bug is flagged as a TODO in the PR description. The Go unit tests (`TestSpecTaskAssigneeSuite`) cover the validation rule and the 400 path; the frontend `yarn build` succeeds.
