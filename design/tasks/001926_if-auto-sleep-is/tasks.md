# Implementation Tasks

- [x] Add `if task.KeepAlive { return nil }` gate at the top of `handleDone()` in `api/pkg/services/spec_task_orchestrator.go:1212`, with an info-level log line.
- [x] Verify `containerExecutor.StopDesktop()` is idempotent — `HydraExecutor.StopDesktop` at `api/pkg/external-agent/hydra_executor.go:561` already logs and continues on "already stopped" errors. `ZedIntegrationService.StopDesktop` at `api/pkg/services/zed_integration_service.go:613` is a no-op. Both safe to call multiple times.
- [x] In the keep-alive update handler at `api/pkg/server/spec_driven_task_handlers.go`, after persisting the `KeepAlive` change, if the previous value was `true`, the new value is `false`, AND `task.Status == TaskStatusDone`, AND `task.PlanningSessionID != ""`, call `s.externalAgentExecutor.StopDesktop()` to release resources.
- [x] Wire `containerExecutor` into the handler — already available as `s.externalAgentExecutor` (used elsewhere in the same file).
- [~] Add a unit test in `api/pkg/services/` covering `handleDone` for both `KeepAlive=true` (asserts `StopDesktop` is NOT called) and `KeepAlive=false` (asserts current behavior). Use `gomock`.
- [ ] Add a unit test for the keep-alive update handler covering the "toggle off on Done task" path.
- [x] Build: `go build ./pkg/server/ ./pkg/services/ ./pkg/store/ ./pkg/types/` — passes.
- [ ] Manual browser test in inner Helix (`localhost:8080`): merge a PR with Keep Alive ON, verify desktop stays running and lock icon remains visible.
- [ ] Manual browser test: merge a PR with Keep Alive OFF, verify existing shutdown behavior unchanged.
- [ ] Manual browser test: on a Done task with running desktop, toggle Keep Alive OFF and verify desktop stops.
- [ ] Frontend `yarn build` to confirm no regressions (no frontend code change expected, but the lock icon visibility on Done tasks should be verified visually).
