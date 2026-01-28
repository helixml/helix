# Implementation Tasks

- [x] In `api/pkg/services/spec_driven_task_service.go` `StartSpecGeneration()`: Replace `task.OriginalPrompt` with `task.Description` when building the agent prompt (~line 400-401)
- [x] In `api/pkg/services/spec_driven_task_service.go` `StartSpecGeneration()`: Update the cloned task case to also use `task.Description` (~line 400)
- [x] In `api/pkg/services/spec_driven_task_service.go` `StartJustDoItMode()`: Replace `task.OriginalPrompt` with `task.Description` in log statement and prompt building (~line 637+)
- [x] Add fallback logic: if `task.Description` is empty, use `task.OriginalPrompt`
- [ ] Manual test: Create task, edit description, start planning, verify agent receives edited description (user verification needed)
- [ ] Manual test: Create task, edit description, start in Just Do It mode, verify agent receives edited description (user verification needed)