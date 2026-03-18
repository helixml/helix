# Implementation Tasks

## Critical (unrecoverable UX)

- [x] **#1a** `hydra_executor.go` `StartDesktop`: add `defer` after setting status `"starting"` that calls `setExternalAgentStatus(context.Background(), sessionID, "")` if the function returns a non-nil error
- [x] **#1b** `hydra_executor.go` `StartDesktop`: give `waitForDesktopBridge` its own 90-second `context.WithTimeout(context.Background(), 90*time.Second)` — decoupled from `dockerCtx` so container creation time doesn't consume the bridge wait budget
- [x] **#4** `hydra_executor.go` `StopDesktop`: add `h.setExternalAgentStatus(ctx, sessionID, "")` and `h.updateSessionStatusMessage(ctx, sessionID, "")` unconditionally before returning `nil`
- [x] **#5** Frontend: find the component that shows "Starting Desktop..." and add a Stop/Restart button visible in the `"starting"` state; add a 2-minute client-side timeout that shows "Desktop may have failed to start — click to retry"

## High (causes agent confusion / duplicate work)

- [x] **#10a** `spec_driven_task_service.go`: before `store.CreateSession` for a spectask, query for an existing session with matching `spec_task_id` + `agent_type=zed_external`; return existing session if found
- [x] **#10b** `prompt_history_handlers.go` `processPendingPromptsForIdleSessions`: use the spectask's `planning_session_id` to target only the canonical session, rather than scanning all sessions by `spec_task_id`
- [x] **#2** Add `ClaimPromptForSending(ctx, promptID) (bool, error)` atomic store method; call it from both the interrupt delivery path (`processInterruptPrompt`) and the `agent_ready` path (`processAnyPendingPrompt`) before sending any chat message — prevents duplicate sends

## Medium (session quality)

- [x] **#7** `devcontainer.go` `resolveDockerDataDir`: in the "no golden anywhere → fresh zvol" branch, acquire a read lock on the golden lock and re-check `GoldenZvolExists` before falling through to `CreateSessionZvol`
- [x] **#3** Already handled by existing `handleAgentReady` code: sends `open_thread` when `agent_ready` arrives with `thread_id == ""` (reconnect/restart), which forces Zed to reload and re-subscribe the thread

## Merge prerequisite

- [x] Merge PR #1947 (`fix/xfs-nouuid-mount`) which contains: `mount -o nouuid`, `DeleteGolden` clone cleanup, `RecoverStaleBuilds` 60s retry; run `./stack build-sandbox` after merge

## Verification

- [x] `go build ./pkg/server/ ./pkg/store/ ./pkg/services/ ./pkg/hydra/ ./pkg/external-agent/` — no compile errors
- [x] TypeScript check on ExternalAgentDesktopViewer.tsx — no errors in our file
- [ ] Start a session on ZFS host, confirm it reaches "running" state and `external_agent_status` clears on stop
- [ ] Trigger a golden promotion while a session is starting — confirm session gets a clone not an empty zvol
- [ ] Send a chat message, restart the session, confirm the message is delivered exactly once
