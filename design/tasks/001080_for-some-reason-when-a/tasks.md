# Implementation Tasks

- [ ] In `api/pkg/server/spec_task_clone_handlers.go`, modify `cloneTaskToProject()` to set `DesignDocsPushedAt` to current time when the source task has specs
- [ ] In `cloneTaskToProject()`, set initial status to `spec_review` instead of `queued_spec_generation` when specs already exist (and autoStart is true)
- [ ] Add unit test in `spec_task_clone_handlers_test.go` to verify cloned tasks with specs get `DesignDocsPushedAt` set
- [ ] Manually test: Clone a task with specs, verify "Review Spec" button appears
- [ ] Manually test: Click "Review Spec" button, verify design review UI loads correctly with the cloned specs