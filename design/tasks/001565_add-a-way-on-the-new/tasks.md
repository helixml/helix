# Implementation Tasks

- [x] Merge latest main into feature branch (per reviewer note: "Just Do It" UI label renamed to "Skip planning")
- [x] Add `AutoStart bool` field to `CreateTaskRequest` in `api/pkg/types/simple_spec_task.go`
- [x] In `CreateTaskFromPrompt` (`api/pkg/services/spec_driven_task_service.go`), set task status to `QueuedSpecGeneration` (or `QueuedImplementation` if `JustDoItMode`) before `store.CreateSpecTask` when `req.AutoStart == true`
- [x] Regenerate `frontend/src/api/api.ts` via `./stack update_openapi` so `TypesCreateTaskRequest.auto_start` appears
- [x] Add `autoStart` state, reset in `resetForm`, and include `auto_start: autoStart` in `createTaskRequest` payload in `NewSpecTaskForm.tsx`
- [x] Add "Start immediately" checkbox UI below the "Skip planning" (formerly "Just Do It") checkbox in `NewSpecTaskForm.tsx`
- [x] Build verification: backend (`go build ./api/pkg/types/ ./api/pkg/services/`) ✅, frontend (`yarn build` + `tsc --noEmit`) ✅
- [x] Write PR description files
