# Implementation Tasks: Smooth Sandbox Restart With Progress and Clean Reconnect

## Backend

- [ ] Add `MarkSessionRestarting(ctx, sessionID)` to `api/pkg/store/store_sessions.go` using the targeted JSONB-merge pattern of `MarkSessionStartingIfIdle` (sets `external_agent_status="restarting"`, `status_message="Restarting desktop…"`)
- [ ] Extend `ClearStaleStartingSessions` and the abandon-boot clear to also match `restarting`, so an API restart mid-teardown cannot pin a session in a spinner
- [ ] In `restartSessionContainer` (`api/pkg/server/session_handlers.go`), mark the session `restarting` before `StopDesktop`, and clear the marker on every error return path
- [ ] In `HydraExecutor.StopDesktop` (`api/pkg/external-agent/hydra_executor.go`), preserve an `external_agent_status` of `restarting` when clearing status at the end of teardown (still clears `running`/`starting`, so Stop-while-starting is unaffected)
- [ ] Extract `liveExternalAgentStatus(session)` implementing the single rule: never downgrade `starting`/`restarting`; probe the executor only when `ContainerName != ""` and the stored status is `running` or empty
- [ ] Replace the inline probe in `getSession` (`session_handlers.go:80-92`) with the shared helper, keeping it ahead of the ETag computation
- [ ] Replace the inline probe in `listTasks` (`spec_driven_task_handlers.go:534-546`) with the shared helper, and map `restarting` → `sandbox_state: "starting"`
- [ ] Add Go unit tests: `liveExternalAgentStatus` table test across status × container-name × executor-tracking; `restartSessionContainer` marks then clears on failure
- [ ] Run `go build ./pkg/server/ ./pkg/store/ ./pkg/types/` and the affected package tests

## Frontend

- [ ] Map `"restarting"` → `"starting"` in `deriveSandboxState` (`frontend/src/components/external-agent/sandboxState.ts`) so every consumer gets the spinner with no per-call-site change
- [ ] Generalise `optimisticallyMarkSessionStarting` to accept `{ status, message, force }`, preserving current behaviour for existing callers
- [ ] Call it with `status: 'restarting', force: true` at the top of `handleRestartSession` (`SpecTaskDetailContent.tsx`), and `invalidateQueries` on failure to drop the optimistic value
- [ ] Keep `restartBusy` true for the whole boot by OR-ing `isRestarting` with `isDesktopStarting`; verify `showStart`/`showStop`/`showRestart` never expose a Start button while restarting (both the split-view and mobile `SpecTaskViewToolbar` call sites)
- [ ] Replace the `sawPausedRef` paused→reachable edge in `ExternalAgentDesktopViewer` with a "re-entered `running` after leaving it" edge so `wakeSignal` fires on `running → starting → running`; keep first-mount `loading → running` from firing
- [ ] Show `statusMessage` (fallback "Restarting desktop…") instead of the hardcoded "Reconnecting…" in the reconnecting overlay
- [ ] Apply the existing `startingTooLong` watchdog to the reconnecting overlay so a wedged restart offers a recovery action instead of an endless spinner
- [ ] Check `OrgAgentSessionWorkspace` / `botSandboxIndicatorState`: confirm a bot restart reports `sandbox_status: pending` for the whole re-provision, and fix it if it reports stopped
- [ ] Add vitest coverage: `deriveSandboxState('restarting')`, the optimistic helper's `force` path, and the `wakeSignal` edge
- [ ] Run `cd frontend && yarn build`

## Verification

- [ ] End-to-end in the inner Helix: register, create a spec task, wait for a live desktop, click Restart, and screenshot at ~2s / ~15s / after reconnect — no "Desktop paused" or Start button at any sample
- [ ] Confirm the stream reattaches to a genuinely new container (`docker ps | grep ubuntu-external-<sid>` shows a different container id) with no manual reconnect or page reload
- [ ] Verify the Kanban card for the same task shows `starting`, not stopped, during the restart window
- [ ] Verify the failure path: a restart that errors after teardown lands on the paused placeholder with the error detail, not a stuck spinner
- [ ] Save the screenshot sequence to this task's `screenshots/` folder and reference it in the PR
