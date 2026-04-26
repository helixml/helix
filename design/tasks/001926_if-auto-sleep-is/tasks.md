# Implementation Tasks

- [x] Add `if task.KeepAlive { return nil }` gate at the top of `handleDone()` in `api/pkg/services/spec_task_orchestrator.go:1212`, with an info-level log line.
- [~] Verify `containerExecutor.StopDesktop()` is idempotent (no panic / no spurious error if called twice for the same session); fix or document if not.
- [ ] In the keep-alive update handler at `api/pkg/server/spec_driven_task_handlers.go:967`, after persisting the `KeepAlive` change, if the new value is `false` AND `task.Status == TaskStatusDone` AND the desktop is still running, call `containerExecutor.StopDesktop()` to release resources.
- [ ] Wire `containerExecutor` into the handler if not already injected (mirror the orchestrator's wiring).
- [ ] Add a unit test in `api/pkg/services/` covering `handleDone` for both `KeepAlive=true` (asserts `StopDesktop` is NOT called) and `KeepAlive=false` (asserts current behavior). Use `gomock`.
- [ ] Add a unit test for the keep-alive update handler covering the "toggle off on Done task" path.
- [ ] Build: `go build ./pkg/server/ ./pkg/services/ ./pkg/store/ ./pkg/types/`.
- [ ] Manual browser test in inner Helix (`localhost:8080`): merge a PR with Keep Alive ON, verify desktop stays running and lock icon remains visible.
- [ ] Manual browser test: merge a PR with Keep Alive OFF, verify existing shutdown behavior unchanged.
- [ ] Manual browser test: on a Done task with running desktop, toggle Keep Alive OFF and verify desktop stops.
- [ ] Frontend `yarn build` to confirm no regressions (no frontend code change expected, but the lock icon visibility on Done tasks should be verified visually).
