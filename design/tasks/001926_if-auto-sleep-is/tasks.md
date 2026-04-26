# Implementation Tasks

- [x] Add `if task.KeepAlive { return nil }` gate at the top of `handleDone()` in `api/pkg/services/spec_task_orchestrator.go:1212`, with an info-level log line.
- [x] Verify `containerExecutor.StopDesktop()` is idempotent — `HydraExecutor.StopDesktop` at `api/pkg/external-agent/hydra_executor.go:561` already logs and continues on "already stopped" errors. `ZedIntegrationService.StopDesktop` at `api/pkg/services/zed_integration_service.go:613` is a no-op. Both safe to call multiple times.
- [x] In the keep-alive update handler at `api/pkg/server/spec_driven_task_handlers.go`, after persisting the `KeepAlive` change, if the previous value was `true`, the new value is `false`, AND `task.Status == TaskStatusDone`, AND `task.PlanningSessionID != ""`, call `s.externalAgentExecutor.StopDesktop()` to release resources.
- [x] Wire `containerExecutor` into the handler — already available as `s.externalAgentExecutor` (used elsewhere in the same file).
- [x] Add a unit test in `api/pkg/services/` covering `handleDone` for both `KeepAlive=true` (asserts `StopDesktop` is NOT called) and `KeepAlive=false` (asserts current behavior). Use `gomock`. — added `TestHandleDone_KeepAliveSkipsStop` in `spec_task_orchestrator_test.go`.
- [x] Add a unit test for the keep-alive update handler covering the "toggle off on Done task" path. — added `spec_driven_task_keep_alive_test.go` with three cases: KeepAlive off on Done (calls StopDesktop), KeepAlive on (doesn't), KeepAlive off on running task (doesn't).
- [x] Build: `go build ./pkg/server/ ./pkg/services/ ./pkg/store/ ./pkg/types/` — passes.
- [x] Frontend `yarn build` — passes (no FE changes needed; verified `isDesktopRunning` derives from `useSandboxState` runtime state, not task status, so lock icon stays visible after merge).
- [ ] WARNING: NOT manually tested end-to-end. Requires real GitHub repo + PR-merge cycle to validate. Behavior is covered by unit tests:
  - `TestHandleDone_KeepAliveSkipsStop` — gate fires when KeepAlive=true.
  - `TestHandleDone_StopsDesktop` — pre-existing test still passes (default behavior unchanged).
  - `TestKeepAliveOff_OnDoneTask_StopsDesktop` — toggle-off-on-Done releases the desktop.
  - `TestKeepAliveOn_OnDoneTask_DoesNotStopDesktop` — toggle-on doesn't fire StopDesktop.
  - `TestKeepAliveOff_OnRunningTask_DoesNotStopDesktop` — toggle-off on non-Done task is a no-op (idle-shutdown handles it).
