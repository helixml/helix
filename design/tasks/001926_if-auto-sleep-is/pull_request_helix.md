# Honor Keep Alive on PR-merge-triggered shutdown

## Summary

When a spec task's PR was merged, the orchestrator transitioned the task to Done and **always** stopped the desktop — bypassing the Keep Alive toggle (green lock icon), which only protected against idle-based shutdown. This made the lock icon misleading: a user who explicitly turned it green would still lose their session as soon as the PR landed, exactly when they may want to keep iterating.

This PR gates the desktop stop in `handleDone()` on `task.KeepAlive`. The merge is still recorded (status, `MergedAt`, `CompletedAt`, golden-build trigger, attention events all unchanged), but the desktop is left running. The user releases it explicitly by toggling Keep Alive off — handled in the keep-alive update endpoint by calling `StopDesktop` when the flag transitions `true → false` on a Done task.

## Changes

- `api/pkg/services/spec_task_orchestrator.go`: gate `StopDesktop` call in `handleDone()` on `task.KeepAlive`.
- `api/pkg/server/spec_driven_task_handlers.go`: capture previous KeepAlive value; after a successful update, if the user just turned KeepAlive off on a Done task, call `s.externalAgentExecutor.StopDesktop()` to release the desktop.
- `api/pkg/services/spec_task_orchestrator_test.go`: add `TestHandleDone_KeepAliveSkipsStop` (asserts `StopDesktop` is NOT called when KeepAlive=true).
- `api/pkg/server/spec_driven_task_keep_alive_test.go`: new test suite covering all three handler paths — toggle-off-on-Done (StopDesktop called), toggle-on (not called), toggle-off on a non-Done task (not called).

## Why gate in `handleDone()`

There are four merge-detection paths in `spec_task_orchestrator.go` (PR poll "all merged", branch-merged-to-main fallback, external PR detection, no-PR fallback). All four set `task.Status = TaskStatusDone` and rely on the state machine to dispatch `handleDone()`. One gate downstream of all four — instead of duplicating `if task.KeepAlive` checks at each site — keeps the change small and avoids breaking the PR-tracking UI (which depends on the status transition).

## Test plan

- [x] Unit tests pass (`TestSpecTaskOrchestratorTestSuite/TestHandleDone_*`, `TestSpecTaskKeepAliveSuite/*`).
- [x] `go build ./pkg/server/ ./pkg/services/ ./pkg/store/ ./pkg/types/` passes.
- [x] `cd frontend && yarn build` passes (no FE changes).
- [ ] **Not manually validated end-to-end** — requires a real GitHub repo + PR-merge cycle. Behavior is covered by the unit tests above. To validate manually: enable Keep Alive on a task with an open PR, merge the PR on GitHub, verify the desktop stays alive and the lock icon remains visible. Then toggle Keep Alive off and verify the desktop stops.
