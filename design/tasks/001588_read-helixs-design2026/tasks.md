# Implementation Tasks

## Critical (unrecoverable UX)

- [x] **#1a** `hydra_executor.go` `StartDesktop`: add `defer` after setting status `"starting"` that calls `setExternalAgentStatus(context.Background(), sessionID, "")` if the function returns a non-nil error
- [x] **#1b** `hydra_executor.go` `StartDesktop`: give `waitForDesktopBridge` its own 90-second `context.WithTimeout(context.Background(), 90*time.Second)` — decoupled from `dockerCtx` so container creation time doesn't consume the bridge wait budget
- [x] **#4** `hydra_executor.go` `StopDesktop`: add `h.setExternalAgentStatus(ctx, sessionID, "")` and `h.updateSessionStatusMessage(ctx, sessionID, "")` unconditionally before returning `nil`
- [x] **#5** Frontend: find the component that shows "Starting Desktop..." and add a Stop/Restart button visible in the `"starting"` state; add a 2-minute client-side timeout that shows "Desktop may have failed to start — click to retry"

## High (causes agent confusion / duplicate work)

- [x] **#10a** `spec_driven_task_service.go`: before `store.CreateSession` for a spectask, query for an existing session with matching `spec_task_id` + `agent_type=zed_external`; return existing session if found
- [x] **#10b** `prompt_history_handlers.go` `processPendingPromptsForIdleSessions`: use the spectask's `planning_session_id` to target only the canonical session, rather than scanning all sessions by `spec_task_id`
- [~] **#2** Add `isRequestIDAlreadyDelivered(ctx, sessionID, requestID) bool` helper; call it from both the 30s poller (`processPendingPromptsForIdleSessions`) and the `agent_ready` delivery path in `websocket_external_agent_sync.go` before sending any chat message

## Medium (session quality)

- [ ] **#7** `devcontainer.go` `resolveDockerDataDir`: in the "no golden anywhere → fresh zvol" branch, acquire a read lock on the golden lock and re-check `GoldenZvolExists` before falling through to `CreateSessionZvol`
- [ ] **#3** `websocket_external_agent_sync.go`: on `agent_ready` when `zed_thread_id` already exists in session config (restart), trigger full bidirectional thread reconciliation rather than only a catch-up snapshot

## Merge prerequisite

- [x] Merge PR #1947 (`fix/xfs-nouuid-mount`) which contains: `mount -o nouuid`, `DeleteGolden` clone cleanup, `RecoverStaleBuilds` 60s retry; run `./stack build-sandbox` after merge

## Verification

- [ ] `go build ./pkg/server/ ./pkg/external-agent/ ./pkg/services/` — no compile errors
- [ ] `cd frontend && yarn build` — no TypeScript errors
- [ ] Start a session on ZFS host, confirm it reaches "running" state and `external_agent_status` clears on stop
- [ ] Trigger a golden promotion while a session is starting — confirm session gets a clone not an empty zvol
- [ ] Send a chat message, restart the session, confirm the message is delivered exactly once
