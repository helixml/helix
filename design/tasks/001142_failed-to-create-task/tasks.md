# Implementation Tasks

## Backend Changes

- [ ] Modify `CreateTaskFromPrompt` in `api/pkg/services/spec_driven_task_service.go` to return existing active task instead of error when `BranchModeExisting` is used and an active task exists on the branch
- [ ] Add `FoundExisting bool` field to `SpecTask` struct in `api/pkg/types/simple_spec_task.go` (with `json:"found_existing,omitempty"`)
- [ ] Update swagger annotations if needed and run `./stack update_openapi`

## Frontend Changes

- [ ] Update `NewSpecTaskForm.tsx` to check for `found_existing` in response and show toast "Resuming existing task: {name}"
- [ ] Update `NewSpecTaskForm.tsx` to navigate to existing task detail page when `found_existing` is true
- [ ] Update `SpecTasksPage.tsx` with same changes (duplicate form exists there)

## Testing

- [ ] Test: "Continue existing" with active task on branch → redirects to that task
- [ ] Test: "Continue existing" with only completed/archived tasks → creates new task
- [ ] Test: "Continue existing" with unused branch → creates new task  
- [ ] Test: "Start fresh" mode still works as before