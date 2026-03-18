# Design: Fix ZFS Deployment Issues

## Key Files

| File | Issues |
|------|--------|
| `api/pkg/external-agent/hydra_executor.go` | #1 (context split, defer cleanup), #4 (stop clears status) |
| `api/pkg/services/spec_driven_task_service.go` | #10 (idempotent session create) |
| `api/pkg/server/prompt_history_handlers.go` | #10 (use planning_session_id only), #2 (dedup by request_id) |
| `api/pkg/server/websocket_external_agent_sync.go` | #2 (dedup by request_id), #3 (full reconcile on restart) |
| `api/pkg/hydra/devcontainer.go` | #7 (promotion race: read-lock before fresh zvol) |
| `frontend/src/components/` (SpecTaskDetailContent or ExternalAgentDesktopViewer) | #5 (restart button) |

## Fix Designs

### #1 — Split `dockerCtx` from bridge context; defer status cleanup

`StartDesktop` currently uses one `ctx` (with 120s timeout from caller) for both `CreateDevContainer` **and** `waitForDesktopBridge`. If the container takes 84s to clone, only 36s remain for the bridge wait. When the bridge needs 53s, the context is already cancelled — but the error path (`setExternalAgentStatus(ctx, id, "")`) is never reached because the error came from `waitForDesktopBridge`, not `CreateDevContainer`.

**Fix**:
1. Add a `defer` immediately after `setExternalAgentStatus(ctx, id, "starting")` that clears the status if the function returns an error:
   ```go
   h.setExternalAgentStatus(ctx, agent.SessionID, "starting")
   var startErr error
   defer func() {
       if startErr != nil {
           h.setExternalAgentStatus(context.Background(), agent.SessionID, "")
       }
   }()
   // ... use startErr = ... for all error returns
   ```
2. Give `waitForDesktopBridge` its own 90-second context detached from `dockerCtx`:
   ```go
   bridgeCtx, bridgeCancel := context.WithTimeout(context.Background(), 90*time.Second)
   defer bridgeCancel()
   if err := h.waitForDesktopBridge(bridgeCtx, agent.SessionID); err != nil {
       log.Warn()... // non-fatal, container is running
   }
   ```
   Do **not** assign this to `startErr` — a bridge timeout is non-fatal (container is running).

### #4 — `StopDesktop` clears `external_agent_status`

Add at the end of `StopDesktop`, before returning `nil`:
```go
h.setExternalAgentStatus(ctx, sessionID, "")
h.updateSessionStatusMessage(ctx, sessionID, "")
```
This is unconditional — even if `DeleteDevContainer` returned an error, the container is likely already gone, and the status must be cleared so the UI reflects reality.

### #5 — Frontend restart button

In the component rendering the desktop section (check `SpecTaskDetailContent.tsx` and `ExternalAgentDesktopViewer.tsx`): show a "Restart Desktop" or "Stop" button whenever `external_agent_status` is `"starting"` or the session has been in that state for >2 minutes. The existing stop/restart API endpoint works regardless of container state. Add a client-side timestamp check: if `"starting"` for >120s, show "Desktop may have failed to start — click to retry".

### #10 — Idempotent spectask session creation

In `spec_driven_task_service.go`, before calling `store.CreateSession` for a spectask:
```go
existingSessions, _ := s.store.ListSessions(ctx, &ListSessionsQuery{
    SpecTaskID: specTaskID,
    AgentType:  "zed_external",
})
if len(existingSessions) > 0 {
    return existingSessions[0], nil
}
```
Also in `processPendingPromptsForIdleSessions`: instead of scanning all sessions by `spec_task_id`, look up the spectask, get `planning_session_id`, and only process that session.

### #2 — Deduplicate messages by `request_id`

Both delivery paths (the 30s poller in `processPendingPromptsForIdleSessions` and the `agent_ready` handler in `websocket_external_agent_sync.go`) must check before sending:
- Query the Zed thread messages for a message with the same `request_id`
- If already present, skip (mark prompt entry as `delivered` not `pending`)

Add a helper `isRequestIDAlreadyDelivered(ctx, sessionID, requestID) bool` that checks the thread messages. Both paths call it.

### #7 — Promotion race

In `devcontainer.go`'s `resolveDockerDataDir`, the `else` branch (no golden zvol, no old golden dir → fresh zvol) runs without taking any lock. If a promotion is in progress, `GoldenZvolExists()` returns false and the session gets an empty zvol.

**Fix**: Acquire a read lock before falling through to fresh zvol creation, then re-check:
```go
} else {
    lock := getGoldenLock(req.ProjectID)
    lock.RLock()
    alreadyExists := GoldenZvolExists(req.ProjectID)
    lock.RUnlock()
    if alreadyExists {
        return SetupGoldenClone(req.ProjectID, sessionID)
    }
    return CreateSessionZvol(sessionID)
}
```

### #3 — Full thread reconciliation on restart

On session restart (detected when `agent_ready` fires and `zed_thread_id` is already set in session config), instead of only sending a catch-up snapshot, perform a bidirectional sync: compare thread messages from Zed against Helix's interaction history and send any missing messages in both directions. This is the existing reconciliation path but needs to be triggered more aggressively on restart.

## Architectural Notes

- `helix-4` is a symlink to `helix` — all edits go in `/home/retro/work/helix/`
- API changes hot-reload via Air; Hydra changes in `api/pkg/hydra/` require `./stack build-sandbox`
- `hydra_executor.go` is API-side (not Hydra), so changes hot-reload
- PR #1947 (`fix/xfs-nouuid-mount`) must be merged before or alongside these fixes; it's already rebased on main

## Codebase Patterns Found

- `setExternalAgentStatus` and `updateSessionStatusMessage` are helpers on `HydraExecutor` at `hydra_executor.go:728+`
- `StopDesktop` does NOT currently call `setExternalAgentStatus` — clear omission confirmed by reading lines 461-516
- The golden lock (`getGoldenLock`) is per-project, used in `devcontainer.go` for golden builds
- `processPendingPromptsForIdleSessions` scans by `spec_task_id` (line 82: `ListPromptHistoryBySpecTask`), not by `planning_session_id` — root cause of issue #10 cascade
