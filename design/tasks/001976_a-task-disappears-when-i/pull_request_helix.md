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
- [x] Manual smoke test in the inner Helix — verified end-to-end:
  - assignee field defaults to the current user (`screenshots/01-new-task-form-default-current-user.png`)
  - popover lists "Unassigned" + members, current user sorted to top (`02-assignee-selector-popover.png`)
  - submitting with the default creates a card with the current user's avatar (`03-task-created-assigned-to-current-user.png`)
  - choosing "Unassigned" before submit creates an unassigned card (`04-task-created-unassigned.png`)
  - **Original bug repro fixed**: with the kanban filter set to "Test User", a freshly-created task (defaulted to current user) appears immediately on the board — no longer disappears (`05-filter-set-to-test-user.png`, `06-bug-fixed-new-task-visible-with-filter.png`)

## Screenshots

![New task form — assignee defaults to current user](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001976_a-task-disappears-when-i/screenshots/01-new-task-form-default-current-user.png)
![Assignee popover with Unassigned + members](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001976_a-task-disappears-when-i/screenshots/02-assignee-selector-popover.png)
![Task created with default assignee](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001976_a-task-disappears-when-i/screenshots/03-task-created-assigned-to-current-user.png)
![Task created with Unassigned](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001976_a-task-disappears-when-i/screenshots/04-task-created-unassigned.png)
![Bug repro: new task visible with active "Test User" filter](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001976_a-task-disappears-when-i/screenshots/06-bug-fixed-new-task-visible-with-filter.png)

## Notes

- The kanban assignee filter remains client-side (`SpecTaskKanbanBoard.tsx`) — backend list filtering by `assignee_id` was deliberately left out of scope (data volumes don't justify it; see design doc).
- The update endpoint still uses `*string` for `AssigneeID` to distinguish "clear" from "no change". Create uses plain `string` because empty already unambiguously means "no assignee".
