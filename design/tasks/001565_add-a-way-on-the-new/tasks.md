# Implementation Tasks

- [ ] Add `AutoStart bool` field to `CreateTaskRequest` in `api/pkg/types/simple_spec_task.go`
- [ ] In `CreateTaskFromPrompt` (`api/pkg/services/spec_driven_task_service.go`), set task status to `QueuedSpecGeneration` (or `QueuedImplementation` if `JustDoItMode`) before `store.CreateSpecTask` when `req.AutoStart == true`
- [ ] Add `auto_start?: boolean` to `TypesCreateTaskRequest` in `frontend/src/api/api.ts` (check if file is generated from swagger; if so, update the swagger source in `api/pkg/server/docs.go` and `swagger/docs.go`)
- [ ] Add `autoStart` state and reset to `NewSpecTaskForm.tsx`
- [ ] Include `auto_start: autoStart` in the `createTaskRequest` payload in `handleCreateTask`
- [ ] Add "Start immediately" checkbox UI below the "Just Do It" checkbox in `NewSpecTaskForm.tsx`
