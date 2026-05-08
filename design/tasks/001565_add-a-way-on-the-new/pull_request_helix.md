# Add "Start immediately" option to new spec task form

## Summary

Adds an opt-in checkbox to the New Spec Task form that lets a user mark a single task to start immediately on creation, bypassing the project's `auto_start_backlog_tasks` setting. This is useful when the project normally uses manual task control but a specific task is urgent.

UX choice: a **checkbox** (mirroring the existing "Skip planning" checkbox) rather than a split button or second submit button. It's always visible, intent is explicit, and the pattern is already familiar from the form's existing layout.

## Changes

- **API** (`api/pkg/types/simple_spec_task.go`): new `AutoStart bool` field on `CreateTaskRequest` with json tag `auto_start`.
- **Service** (`api/pkg/services/spec_driven_task_service.go`): `CreateTaskFromPrompt` now sets the initial status to `TaskStatusQueuedSpecGeneration` (or `TaskStatusQueuedImplementation` if `JustDoItMode` is also set) when `AutoStart` is true, mirroring the existing `cloneTaskToProject` pattern. Status is set before `store.CreateSpecTask` so the orchestrator picks it up on the first poll.
- **UI** (`frontend/src/components/tasks/NewSpecTaskForm.tsx`): new `autoStart` state, included in the create payload, reset on form reset, and rendered as a "Start immediately" checkbox (primary colour) directly below the "Skip planning" checkbox.
- **Generated client**: regenerated `openapi.json`, `swagger.json`, swagger YAMLs, and `frontend/src/api/api.ts` via `./stack update_openapi`.

## Behaviour

- Default: unchecked → unchanged behaviour, task lands in backlog.
- Checked + Skip planning unchecked → task starts spec generation immediately.
- Checked + Skip planning checked → task starts implementation immediately.
- The checkbox is visible regardless of the project's auto-start setting; it's a per-task override.

## Test plan

- [ ] Create a task with the box unchecked → confirm it lands in `backlog`.
- [ ] Create a task with the box checked → confirm it lands in `queued_spec_generation` and the orchestrator picks it up.
- [ ] Create a task with both "Start immediately" and "Skip planning" checked → confirm it lands in `queued_implementation`.
- [ ] Confirm the checkbox state resets after submission.

Spec: [helix-specs/design/tasks/001565_add-a-way-on-the-new/](https://github.com/helixml/helix/tree/helix-specs/design/tasks/001565_add-a-way-on-the-new)
