# Implementation Tasks

- [~] Add `AssigneeID string` field (json `assignee_id,omitempty`) to `CreateTaskRequest` in `api/pkg/types/simple_spec_task.go`
- [ ] Extract the existing assignee org-membership validation from `updateSpecTask` (`api/pkg/server/spec_driven_task_handlers.go:1053-1075`) into a shared helper (e.g. `validateAssigneeIsOrgMember`)
- [ ] Call the helper from `createTaskFromPrompt` after authorising the user; return HTTP 400 with `"assignee must be an organization member"` on failure
- [ ] In `specDrivenTaskService.CreateTaskFromPrompt` (`api/pkg/services/spec_driven_task_service.go:174-196`), copy `req.AssigneeID` into the new `SpecTask.AssigneeID`
- [ ] Run `./stack update_openapi` to regenerate the frontend API client with the new `assignee_id` field
- [ ] In `frontend/src/components/tasks/NewSpecTaskForm.tsx`, add an assignee field near the priority selector that reuses `AssigneeSelector`, defaulting to the current user, sourcing `members`/`currentUserId` from the account context (same pattern as `TaskCard.tsx:564-570`)
- [ ] Include `assignee_id` in the body sent to `v1SpecTasksFromPromptCreate` (around line 351 of `NewSpecTaskForm.tsx`)
- [ ] Add Go test cases for the create endpoint: no assignee, valid assignee, invalid (non-member) assignee
- [ ] Manually verify in the inner Helix: create task with default assignee, change to another member, change to Unassigned; reproduce the original bug (assignee filter set to current user, create task) and confirm the new task is visible
- [ ] Update design docs in this folder if the implementation deviates from the plan
