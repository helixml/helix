# ZFS Clone Deployment Issues — 2026-03-18

Issues observed during first production deployment of ZFS zvol cloning on meta.helix.ml.

## 1. Session stuck in "Starting Desktop" forever

**Symptom**: Frontend shows "Starting Desktop..." with no way to recover. Hard refresh doesn't help. No restart button visible in this state.

**Root cause**: The `resumeSession` handler sets `external_agent_status = "starting"` before calling `StartDesktop`. `StartDesktop` calls `CreateDevContainer` which has a 120-second Docker API timeout (`dockerCtx`). When the ZFS clone took 84 seconds (due to concurrent Docker container kill contention), only 36 seconds remained for `waitForDesktopBridge`. The bridge took 53 seconds to connect. By then the context was cancelled.

The error path at `hydra_executor.go:380` (`setExternalAgentStatus(ctx, sessionID, "")`) didn't fire because the error was `context canceled` from a different code path (the `waitForDesktopBridge` wrapper, not `CreateDevContainer` itself). The status stayed as `"starting"` in the DB permanently.

**Impact**: User cannot interact with the session. The agent IS running (messages flowing, work happening) but the video stream and desktop UI are invisible. The only fix was manual DB update to clear `external_agent_status`.

**Fix needed**:
- `waitForDesktopBridge` should not use the same context as `CreateDevContainer`. The bridge connect timeout should be independent of the Docker API timeout.
- The `external_agent_status` should be cleared in a `defer` that runs regardless of which path errors.
- Frontend should show a "Restart" or "Reconnect" button even in the "Starting Desktop" state so users can recover without admin intervention.

**Relevant logs**:
```
10:40:18  Creating dev container via Hydra
10:41:44  Dev container created successfully (84s clone under contention)
10:41:44  Desktop bridge not ready (continuing anyway): context canceled
10:41:44  Failed to re-fetch session after StartDesktop: context canceled
10:42:37  ✅ RevDial control connection established  ← 53s later, too late
```

## 2. Duplicate message sends after session restart

**Symptom**: The same `chat_message` with the same `request_id` is sent to Zed twice after a session restart.

**Observed**: `request_id=int_01km08btk5qa7h6ss52072x8qp` was queued at 11:08:04 AND 11:09:12 — same request_id, two sends.

**Likely cause**: The `processPendingPromptsForIdleSessions` poller (runs every 30s) finds the pending prompt and sends it, but the session restart flow also triggers the initial message send. Both fire before the prompt is marked as delivered. Or the prompt queue doesn't deduplicate by request_id.

**Fix needed**: Deduplicate by `request_id` before sending. If a message with the same `request_id` has already been sent (check Zed thread), skip it.

## 3. Session out of sync with Zed thread

**Symptom**: After multiple stop/start cycles, the Zed thread state and the Helix session state diverge. The session shows old messages or the thread has messages that Helix doesn't know about.

**Root cause**: When a session stops, the Zed WebSocket disconnects and the thread state in Zed is frozen. When the session restarts, Helix sends a catch-up snapshot, but this may not account for messages that were in flight during the stop. The `zed_thread_id` persists across restarts (stored in session config), so the thread accumulates state across multiple lifecycle cycles.

**Fix needed**: Investigation into whether the catch-up snapshot is complete, and whether messages sent during the stop window are lost or duplicated.

## 4. `external_agent_status` not cleared on stop

**Symptom**: After clicking "Stop" in admin panel, `external_agent_status` remains `"starting"` in the DB. Container is stopped but UI shows "Starting Desktop" on page refresh.

**Root cause**: `StopDesktop` in `hydra_executor.go` calls `DeleteDevContainer` which stops the container successfully. But it doesn't explicitly clear `external_agent_status`. The status was set to `"starting"` by the original `StartDesktop` call and never transitioned to `"running"` (because the context was cancelled). `StopDesktop` doesn't know the status is stuck.

**Fix needed**: `StopDesktop` should unconditionally clear `external_agent_status` and `status_message` after successfully stopping the container.

## 5. No "Restart" button when stuck in "Starting Desktop"

**Symptom**: When the frontend shows "Starting Desktop..." there is no way for the user to recover. No restart, stop, or cancel button is visible.

**Fix needed**: Frontend `SpecTaskDetailContent.tsx` or `ExternalAgentDesktopViewer.tsx` should show a restart/cancel button in all session states, not just "running". A timeout (e.g. 2 minutes) after which the UI shows "Desktop may have failed to start — click to retry" would also help.

## 6. `GoldenBuildService.RecoverStaleBuilds` resets status too quickly

**Symptom**: After API restart (Air hot-reload), `docker_cache_status` in the DB gets reset from `"building"` to `"none"` even though the golden build is still running in the sandbox.

**Root cause**: `RecoverStaleBuilds` calls `HasRunningContainer` which goes through RevDial. After an API restart, the sandbox hasn't reconnected yet (RevDial WebSocket re-establishment takes 5-10 seconds). `HasRunningContainer` returns false → status reset to "none".

**Fix applied**: PR #1947 — retry 12 times with 5s sleep (60s total) before giving up.

## 7. Golden build promotion race — sessions get "no golden cache"

**Symptom**: Sessions starting during `PromoteSessionToGoldenZvol` get `"Using fresh ZFS zvol for session (no golden cache exists)"` even though the golden is being promoted at that exact moment.

**Root cause**: `PromoteSessionToGoldenZvol` holds the golden write lock, which blocks `SetupGoldenClone` and `MigrateGoldenToZvol`. But sessions that hit the `else` branch (`CreateSessionZvol` — no golden exists) don't take the lock. During the brief window where the old golden is destroyed and the new one isn't snapshotted yet, `GoldenZvolExists()` returns false and the session gets an empty zvol.

**Observed timeline**:
```
10:37:00  Golden build completes, starts promoting (acquires lock)
10:37:00  ses_01km011d starts — GoldenZvolExists()=false → empty zvol
10:38:00  Promote finishes → gen4 snapshot created (releases lock)
```

**Fix needed**: The "no golden zvol" path should also check `GoldenZvolExists` under the golden lock, or wait if a promotion is in progress.

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

**Mitigation**: Not clear. Docker's `ContainerStop` with a 2-second timeout is already aggressive. May need to investigate whether the container has processes that resist SIGKILL (e.g. D-state I/O waits on the zvol being unmounted during promotion).

## PR Status

**PR #1947** (`fix/xfs-nouuid-mount`) — OPEN, 6 commits:
1. `mount -o nouuid` instead of `xfs_admin` for XFS cloned zvols
2. `DeleteGolden` destroys session clones before golden zvol
3. `DeleteGolden` refuses if any clones are mounted (running sessions)
4. `RecoverStaleBuilds` waits 60s for sandbox reconnect
5. All tested on meta.helix.ml — gen4 promotion working, sessions cloning successfully

**Not yet merged**: needs review and merge, then `./stack build-sandbox` to deploy the sandbox changes (Hydra binary). The API changes (golden build service) are already hot-reloaded via Air.
