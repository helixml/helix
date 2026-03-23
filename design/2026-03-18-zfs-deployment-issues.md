# ZFS Clone Deployment Issues — 2026-03-18

Issues observed during first production deployment of ZFS zvol cloning on meta.helix.ml. Each issue has root cause analysis, affected files, and specific fix instructions.

## 1. Session stuck in "Starting Desktop" forever

**Symptom**: Frontend shows "Starting Desktop..." with no way to recover. Hard refresh doesn't help. No restart button visible in this state.

**Root cause**: The `resumeSession` handler sets `external_agent_status = "starting"` before calling `StartDesktop`. `StartDesktop` calls `CreateDevContainer` which has a 120-second Docker API timeout (`dockerCtx`). When the ZFS clone took 84 seconds (due to concurrent Docker container kill contention), only 36 seconds remained for `waitForDesktopBridge`. The bridge took 53 seconds to connect. By then the context was cancelled.

The error path at `hydra_executor.go:380` (`setExternalAgentStatus(ctx, sessionID, "")`) didn't fire because the error was `context canceled` from a different code path (the `waitForDesktopBridge` wrapper, not `CreateDevContainer` itself). The status stayed as `"starting"` in the DB permanently.

**Impact**: User cannot interact with the session. The agent IS running (messages flowing, work happening) but the video stream and desktop UI are invisible. The only fix was manual DB update to clear `external_agent_status`.

**Relevant logs**:
```
10:40:18  Creating dev container via Hydra
10:41:44  Dev container created successfully (84s clone under contention)
10:41:44  Desktop bridge not ready (continuing anyway): context canceled
10:41:44  Failed to re-fetch session after StartDesktop: context canceled
10:42:37  ✅ RevDial control connection established  ← 53s later, too late
```

**Files to change**:
- `api/pkg/external-agent/hydra_executor.go`
  - Line ~340: `setExternalAgentStatus(ctx, agent.SessionID, "starting")` — wrap in a defer that clears on ANY error, not just `CreateDevContainer` errors
  - Line ~371-381: `CreateDevContainer` call — the `dockerCtx` (120s timeout) is used for both container creation AND `waitForDesktopBridge`. These should be separate contexts. The container creation can take a while (ZFS clone + Docker pull), but the bridge wait is independent
  - Line ~398: `waitForDesktopBridge` — should use its own context with a separate timeout (e.g. 60s), not the same `dockerCtx` that was partially consumed by container creation
- `api/pkg/server/session_handlers.go`
  - Line ~1916-1924: `resumeSession` handler — if `StartDesktop` returns `context canceled`, the session is actually running (container started, bridge not yet connected). The handler should still update the session metadata and return success, not propagate the context error

**Fix approach**:
```go
// In StartDesktop, use defer to guarantee status cleanup:
h.setExternalAgentStatus(ctx, agent.SessionID, "starting")
defer func() {
    if err != nil {
        h.setExternalAgentStatus(context.Background(), agent.SessionID, "")
    }
}()

// Separate context for bridge wait:
bridgeCtx, bridgeCancel := context.WithTimeout(context.Background(), 60*time.Second)
defer bridgeCancel()
if err := h.waitForDesktopBridge(bridgeCtx, agent.SessionID); err != nil {
    log.Warn()... // Don't fail the session
}
```

## 2. Duplicate message sends after session restart

**Symptom**: The same `chat_message` with the same `request_id` is sent to Zed twice after a session restart.

**Observed**: `request_id=int_01km08btk5qa7h6ss52072x8qp` was queued at 11:08:04 AND 11:09:12 — same request_id, two sends.

**Root cause**: Two delivery paths race to send the same prompt:
1. The session restart flow in `StartDesktop` triggers `processPendingPromptsForIdleSessions` which finds the pending prompt and sends it
2. The readiness queue in `websocket_external_agent_sync.go` also queues the same prompt when the agent sends `agent_ready`

Neither path checks if the other already sent the message. No deduplication by `request_id`.

Additionally, issue #10 (two sessions for one spectask) may cause the prompt scanner to find the pending prompt on both sessions.

**Files to change**:
- `api/pkg/server/prompt_history_handlers.go`
  - `processPendingPromptsForIdleSessions` (line ~79) — before sending, check if the `request_id` has already been delivered to the Zed thread. Query the thread messages for a matching `request_id`
- `api/pkg/server/websocket_external_agent_sync.go`
  - Line ~448: `Queued initial chat_message for Zed` — before queuing, check if this `request_id` was already sent in the current thread
  - Line ~1665: readiness queue — same dedup check

**Fix approach**: Add a `isRequestAlreadySent(sessionID, requestID)` check that queries the Zed thread messages. Both delivery paths call this before sending.

## 3. Session out of sync with Zed thread after restart

**Symptom**: After stop/start cycles, the Zed thread state and the Helix session state diverge. Spec docs that were previously visible become invisible.

**Root cause**: When a session stops, the Zed WebSocket disconnects and the thread state in Zed is frozen. When the session restarts, Helix sends a catch-up snapshot (`websocket_server_user.go:145 Sent late-joiner catch-up snapshot`), but this may not account for messages that were in flight during the stop. The `zed_thread_id` persists across restarts (stored in session config), so the thread accumulates state across multiple lifecycle cycles.

The sync self-healed after the next message was sent — the new message triggered a full thread reconciliation that brought both sides back into alignment.

**Files to change**:
- `api/pkg/server/websocket_external_agent_sync.go`
  - The catch-up snapshot logic (around `Sent late-joiner catch-up snapshot`) — on session restart, force a full state reconciliation rather than just sending a snapshot. This should include checking the thread's current state and sending any missing messages

**Fix approach**: On session restart (detected by `agent_ready` event after a session that previously had a thread), do a full bidirectional sync rather than just a catch-up snapshot.

## 4. `external_agent_status` not cleared on stop

**Symptom**: After clicking "Stop" in admin panel, `external_agent_status` remains `"starting"` in the DB. Container is stopped but UI shows "Starting Desktop" on page refresh.

**Root cause**: `StopDesktop` in `hydra_executor.go` calls `DeleteDevContainer` which stops the container successfully. But it doesn't explicitly clear `external_agent_status`. The status was set to `"starting"` by the original `StartDesktop` call and never transitioned to `"running"` (because the context was cancelled). `StopDesktop` doesn't know the status is stuck.

**Files to change**:
- `api/pkg/external-agent/hydra_executor.go`
  - `StopDesktop` function (line ~461) — after successfully stopping the container, unconditionally clear the agent status:
    ```go
    h.setExternalAgentStatus(ctx, sessionID, "")
    h.updateSessionStatusMessage(ctx, sessionID, "")
    ```

## 5. No "Restart" button when stuck in "Starting Desktop"

**Symptom**: When the frontend shows "Starting Desktop..." there is no way for the user to recover. No restart, stop, or cancel button is visible.

**Files to change**:
- `frontend/src/components/tasks/SpecTaskDetailContent.tsx` — the desktop viewer section that shows "Starting Desktop..." should also render a "Cancel" or "Restart" button
- `frontend/src/components/external-agent/ExternalAgentDesktopViewer.tsx` — same
- Consider adding a client-side timeout: if "Starting Desktop" persists for >2 minutes, show "Desktop may have failed to start" with a retry button

**Fix approach**: Always show a stop/restart affordance regardless of session state. The backend `StopDesktop` endpoint works even when status is stuck.

## 6. `GoldenBuildService.RecoverStaleBuilds` resets status too quickly

**Symptom**: After API restart (Air hot-reload), `docker_cache_status` in the DB gets reset from `"building"` to `"none"` even though the golden build is still running in the sandbox.

**Root cause**: `RecoverStaleBuilds` calls `HasRunningContainer` which goes through RevDial. After an API restart, the sandbox hasn't reconnected yet (RevDial WebSocket re-establishment takes 5-10 seconds). `HasRunningContainer` returns false → status reset to "none".

**Fix applied**: PR #1947 — `api/pkg/services/golden_build_service.go` `RecoverStaleBuilds` now retries 12 times with 5s sleep (60s total) before giving up.

**Status**: ✅ Fixed, in PR #1947.

## 7. Golden build promotion race — sessions get "no golden cache"

**Symptom**: Sessions starting during `PromoteSessionToGoldenZvol` get `"Using fresh ZFS zvol for session (no golden cache exists)"` even though the golden is being promoted at that exact moment.

**Root cause**: `PromoteSessionToGoldenZvol` holds the golden write lock, which blocks `SetupGoldenClone` and `MigrateGoldenToZvol`. But sessions that hit the `else` branch (`CreateSessionZvol` — no golden exists) don't take the lock. During the brief window where the old golden is destroyed and the new one isn't snapshotted yet, `GoldenZvolExists()` returns false and the session gets an empty zvol.

**Observed timeline**:
```
10:37:00  Golden build completes, starts promoting (acquires lock)
10:37:00  ses_01km011d starts — GoldenZvolExists()=false → empty zvol
10:38:00  Promote finishes → gen4 snapshot created (releases lock)
```

**Files to change**:
- `api/pkg/hydra/devcontainer.go`
  - `resolveDockerDataDir` — the `else` branch (no golden zvol, no old golden dir, create fresh zvol) should take a read lock on the golden lock first. If a promotion is in progress (write lock held), it will block until the promotion completes, then find the new golden zvol
  - Alternatively, check `GoldenZvolExists` again after `CreateSessionZvol` — if a golden appeared while we were creating the fresh zvol, destroy the fresh one and clone from golden instead

**Fix approach**:
```go
} else {
    // Take read lock to wait for any in-progress promotion
    lock := getGoldenLock(req.ProjectID)
    lock.RLock()
    lock.RUnlock()
    // Re-check after potential promotion
    if GoldenZvolExists(req.ProjectID) {
        return SetupGoldenClone(req.ProjectID, sessionID)
    }
    // Genuinely no golden — create fresh zvol
    return CreateSessionZvol(sessionID)
}
```

## 8. Docker container kill contention slows ZFS operations

**Symptom**: ZFS clone took 84 seconds instead of the usual 3-12 seconds. Not I/O related (NVMe at 0.1% util).

**Root cause**: Docker was stuck killing the golden build container (`"tried to kill container, but did not receive an exit event"`). This kernel-level container teardown appears to hold resources that slow concurrent ZFS operations (possibly related to mount namespace cleanup or cgroup teardown contending with zvol device creation).

**Observed timeline**:
```
10:38  Stopping golden build container
10:39  "tried to kill container, but did not receive an exit event"
10:40  ZFS clone starts (should take 3s)
10:41  ZFS clone finishes (took 84s)
```

**Files to change**: No immediate code fix — this is a kernel-level contention issue between Docker's container teardown and ZFS zvol operations. However:
- `api/pkg/hydra/devcontainer.go` — `monitorGoldenBuild` calls `DeleteDevContainer` to stop the golden build container. The container uses a zvol that was just unmounted during promotion. If the zvol unmount hasn't fully released kernel resources before Docker tries to kill the container, both operations contend
- Consider adding a small delay between zvol unmount (in `PromoteSessionToGoldenZvol`) and container stop (in `monitorGoldenBuild`) to let kernel resources drain

**Mitigation**: Investigate whether the golden build container's inner dockerd has processes in D-state (uninterruptible I/O wait) on the zvol that was just unmounted. `docker inspect` the container for process state during the kill timeout.

## 9. Spectask stuck in planning — agent doing implementation

**Symptom**: Spectask in "Planning" column with no "Review Spec" button. Agent is actively doing implementation work (editing Go files, operator controllers) but task status is `spec_generation`.

**Root cause**: Cascade of issues #1 → #2 → #10. The duplicate message send (issue #2, caused by issue #10's duplicate sessions) sent the same user comment twice. The agent interpreted the second send as a new instruction and jumped from spec writing into implementation, bypassing the `spec_generation` → `spec_review` → `approved` → `implementation` flow.

The task eventually self-resolved when the agent pushed design docs to git, triggering the auto-transition to `spec_review` at 11:18.

**Files to change**: Fixing issues #2 and #10 should prevent this. Additionally:
- `api/pkg/services/spec_driven_task_service.go` — consider adding a manual "Move to Review" button in the UI for admin recovery, in case tasks get stuck in `spec_generation`
- The agent's system prompt for `spec_generation` includes "Do NOT implement anything yet" but this guardrail failed because the duplicate message overrode the phase context

## 10. Two sessions created for one spectask (race condition)

**Symptom**: Spectask `spt_01kg9w069xs5bfkmxbp7d5y05p` has two sessions created 1 second apart:
- `ses_01kkv1p6j` (10:04:57) — name "project update:...", `spec_task_id` set, no `agent_type`
- `ses_01kkv1p7n` (10:04:58) — name "Spec Generation:...", `spec_task_id` set, `agent_type=zed_external`

Both sessions have `spec_task_id` pointing to the same spectask. The spectask's `planning_session_id` correctly points to the second one. The first has incomplete metadata (no `agent_type`).

**Root cause**: Race condition in session creation. When a user clicks to start a spectask, two code paths fire concurrently — likely the spectask start flow and a project-level update flow — both creating sessions with the same `spec_task_id` before either can check if one already exists.

**Impact**: `processPendingPromptsForIdleSessions` scans sessions by `spec_task_id` and finds both. This causes duplicate prompt delivery (issue #2), which cascades into the agent confusion (issue #9).

**Files to change**:
- `api/pkg/services/spec_driven_task_service.go` — session creation for spectasks. Add a check: if a session with this `spec_task_id` already exists (query by `config->>'spec_task_id'`), return the existing session instead of creating a new one
- `api/pkg/server/spec_driven_task_handlers.go` — the handler that triggers spectask start. May need a per-spectask mutex to prevent concurrent session creation
- `api/pkg/server/prompt_history_handlers.go` — `processPendingPromptsForIdleSessions` should only use the session referenced by `planning_session_id`, not scan all sessions with a matching `spec_task_id`

**Fix approach**:
```go
// Before creating a new session for a spectask:
existingSessions, _ := store.ListSessions(ctx, &ListSessionsQuery{
    SpecTaskID: specTaskID,
    AgentType:  "zed_external",
})
if len(existingSessions) > 0 {
    return existingSessions[0], nil // reuse existing
}
```

## 11. Zed multi-thread support broken for zed-agent runtime

**Symptom**: When using zed-agent (not claude), the agent creates new threads manually (Zed doesn't auto-compact — user must create a new thread). The new thread's activity doesn't sync to Helix. Interacting from Helix UI sends messages to the old thread, causing out-of-context errors.

**Root cause (multiple issues)**:

### 11a. `user_created_thread` event never fires
Zed sends `UserCreatedThread` from `thread_view.rs:999` but only when `entry_count > 0`. For a brand new empty thread, the count is 0 and the event is suppressed. The event also requires `!is_resume` — resumes don't trigger it.

**Files**: `zed/crates/agent_ui/src/acp/thread_view.rs:991-1007`

**Fix**: Remove the `entry_count > 0` guard or send the event after the first message is added to the new thread.

### 11b. New session missing spectask metadata
`handleUserCreatedThread` (`websocket_external_agent_sync.go:3093-3110`) creates a new Helix session but doesn't copy:
- `SpecTaskID` — new session isn't associated with the spectask
- `ProjectID` — needed for project-level operations
- `CodeAgentRuntime` — needed to know zed-agent vs claude
- `DevContainerID` — needed for container association

**Fix**:
```go
Metadata: types.SessionMetadata{
    ZedThreadID:         acpThreadID,
    AgentType:           existingSession.Metadata.AgentType,
    ExternalAgentConfig: existingSession.Metadata.ExternalAgentConfig,
    SpecTaskID:          existingSession.Metadata.SpecTaskID,     // ADD
    CodeAgentRuntime:    existingSession.Metadata.CodeAgentRuntime, // ADD
    ProjectID:           existingSession.Metadata.ProjectID,       // ADD (if it exists)
},
ProjectID: existingSession.ProjectID,  // ADD - top-level project association
```

### 11c. No UI for multiple sessions per spectask
The frontend only shows one session per spectask. When a new thread creates a new session, there's no dropdown or tab to switch between them. The user can't see the new thread's activity from Helix.

**Files**: `frontend/src/components/tasks/SpecTaskDetailContent.tsx` — needs a session selector when multiple sessions exist for the same spectask.

### 11d. `open_thread` on reconnect goes to wrong thread
When the desktop restarts and reconnects, Helix sends `open_thread` with the `ZedThreadID` from the original session. If the user was on a newer thread, Zed jumps back to the old one.

**Files**: `websocket_external_agent_sync.go:2676-2691` — the readiness handler should check which thread is currently active in Zed (or at least use the latest session's thread ID, not the first one's).

### 11e. No E2E test for multi-thread flow
The E2E test (`zed/crates/external_websocket_sync/e2e-test/`) only tests single-thread flows. Need a test phase for: create thread → send messages → create new thread → verify new session created → send message on new thread → verify it goes to new session.

**Observed data**:
- Spectask `spt_01kj8pxf9w49ek6zcdgdckvspe` has only 1 session in DB
- Thread ID `8b975c94-9916-47ca-90ed-511ed8d63dbd` — the original thread
- zed-agent runtime — uses manual thread creation (no auto-compaction)
- No `user_created_thread` events in API logs ever
- User created new thread in Zed UI but Helix never learned about it

## PR Status

**PR #1947** (`fix/xfs-nouuid-mount`) — OPEN, 8 commits:
1. `mount -o nouuid` instead of `xfs_admin` for XFS cloned zvols
2. `DeleteGolden` destroys session clones before golden zvol
3. `DeleteGolden` refuses if any clones are mounted (running sessions)
4. `RecoverStaleBuilds` waits 60s for sandbox reconnect
5. Design doc (this file)
6. All tested on meta.helix.ml — gen4 promotion working, sessions cloning successfully

**Needs merge + rebuild**: The sandbox needs `./stack build-sandbox` after merge for the Hydra changes (nouuid mount, DeleteGolden fixes). The API changes (RecoverStaleBuilds) are already hot-reloaded via Air.

## Summary: Fix Priority

| # | Issue | Severity | Fix complexity | Blocks users? |
|---|-------|----------|----------------|---------------|
| 1 | Stuck "Starting Desktop" | **Critical** | Medium | Yes — unrecoverable without DB fix |
| 4 | Status not cleared on stop | **Critical** | Simple | Yes — compounds issue #1 |
| 5 | No restart button | **Critical** | Simple (frontend) | Yes — no user recovery path |
| 10 | Duplicate sessions for spectask | **High** | Medium | Causes issues #2 and #9 |
| 2 | Duplicate message sends | **High** | Medium | Confuses agent, skips phases |
| 7 | Promotion race (empty zvol) | **Medium** | Simple | Session starts cold |
| 3 | Thread sync after restart | **Medium** | Complex | Self-heals on next message |
| 9 | Spectask stuck in planning | **Medium** | N/A (fixed by #2, #10) | Resolved by fixing root causes |
| 6 | RecoverStaleBuilds too fast | **Low** | ✅ Fixed | Only during API restarts |
| 8 | Docker kill contention | **Low** | Investigation | Rare, temporary slowdown |
