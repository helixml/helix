# Allow setting an assignee when creating a spec task

## Summary

Fixes the bug where a freshly-created task vanishes from the kanban board when the user has an assignee filter active. Previously the create-task endpoint had no way to set an assignee, so every new task started with `assignee_id = ""`. The board's client-side filter (`SpecTaskKanbanBoard.tsx`) treats `""` as `__unassigned__` and only shows it when the user has explicitly selected "Unassigned" in the filter — so any active filter would silently hide the new task.

The create-task form now offers an optional assignee field (defaulting to the current user, with "Unassigned" as a peer option). The selected ID flows through `CreateTaskRequest` → `SpecDrivenTaskService.CreateTaskFromPrompt` → DB. The handler validates that the assignee is a member of the project's organization, mirroring the rule already enforced on update.

## Changes

- `api/pkg/types/simple_spec_task.go` — add optional `AssigneeID` to `CreateTaskRequest`
- `api/pkg/server/spec_driven_task_handlers.go`
  - extract the existing assignee org-membership check from `updateSpecTask` into a shared `validateAssigneeIsOrgMember` helper
  - call the helper from `createTaskFromPrompt` after authorization (HTTP 400 on a non-member assignee, matching the update path)
- `api/pkg/services/spec_driven_task_service.go` — copy `req.AssigneeID` onto the new `SpecTask`
- `frontend/src/components/tasks/NewSpecTaskForm.tsx` — add an assignee selector below the priority picker, reusing `AssigneeSelector`. Defaults to the current user once known; an `assigneeTouched` flag prevents an explicit user choice (including "Unassigned") from being clobbered if the user object loads after first render
- `api/pkg/server/spec_driven_task_assignee_test.go` — new test suite covering the helper (empty / member / non-member) and the handler-level 400 path
- regenerated swagger / TypeScript client to expose `assignee_id` on `CreateTaskRequest`

## Test plan

- [x] `go test -v -run TestSpecTaskAssigneeSuite ./api/pkg/server/` — passes locally (4/4)
- [x] `cd frontend && yarn build` — passes
- [ ] **Not tested in this environment** (no running inner Helix to drive a browser against): manual smoke test in the inner Helix —
  - register/log in, open the New Task panel
  - confirm the assignee field defaults to the current user
  - change to another org member, submit, confirm the new card shows that assignee
  - open the form again, change the assignee to "Unassigned", submit, confirm the new card is unassigned
  - set the kanban filter to "current user only", create a task with the default, confirm it lands in the visible columns immediately (the original bug repro)
  - try to assign to a non-member via the API directly, confirm HTTP 400 with `assignee must be an organization member`

## Notes

- The kanban assignee filter remains client-side (`SpecTaskKanbanBoard.tsx`) — backend list filtering by `assignee_id` was deliberately left out of scope (data volumes don't justify it; see design doc).
- The update endpoint still uses `*string` for `AssigneeID` to distinguish "clear" from "no change". Create uses plain `string` because empty already unambiguously means "no assignee".
