# Requirements: Fix ZFS Deployment Issues

Source: `design/2026-03-18-zfs-deployment-issues.md` — 10 issues observed during first production deployment of ZFS zvol cloning on meta.helix.ml.

## User Stories

**As a user with a session stuck in "Starting Desktop"**, I want a Restart button so I can recover without admin intervention.

**As a developer**, I want `external_agent_status` cleared whenever a desktop stops or fails to start, so the UI never shows a stale "Starting Desktop" permanently.

**As a spectask user**, I want my message sent to the agent exactly once, so the agent doesn't receive duplicate instructions that cause it to skip lifecycle phases (spec → review → approved → implementation).

**As a system**, I want session creation for spectasks to be idempotent so a race condition can't create two sessions for the same task.

**As a session starting on a ZFS host**, I want to wait for an in-progress golden promotion rather than getting an empty zvol, so I start with a warm build cache.

## Acceptance Criteria

### Issue #1 — Stuck "Starting Desktop" (Critical)
- If `waitForDesktopBridge` fails or times out, `external_agent_status` is cleared and the session is not left in permanent `"starting"` state
- `waitForDesktopBridge` uses its own independent context (not the partially-consumed `dockerCtx`)
- A session whose container started successfully but bridge timed out is usable after reconnect

### Issue #4 — Status not cleared on stop (Critical)
- After `StopDesktop` returns, `external_agent_status` is always empty regardless of what it was before

### Issue #5 — No restart button (Critical)
- Frontend shows a Stop/Restart button in the "Starting Desktop" state, not only in the "running" state
- If "Starting Desktop" persists >2 minutes, UI shows a user-visible error message with a retry prompt

### Issue #10 — Duplicate sessions per spectask (High)
- Creating a spectask session when one already exists for that `spec_task_id` + `agent_type` returns the existing session instead of creating a new one
- `processPendingPromptsForIdleSessions` only targets the session referenced by `planning_session_id`, not all sessions with a matching `spec_task_id`

### Issue #2 — Duplicate message sends (High)
- A chat message with a given `request_id` is sent to the Zed thread at most once, even if two delivery paths race

### Issue #7 — Promotion race gives empty zvol (Medium)
- A session starting while `PromoteSessionToGoldenZvol` holds the golden write lock either waits for promotion to complete or re-checks for the golden zvol after acquiring a read lock, and always gets an instant clone when golden exists

### Issue #3 — Thread out of sync after restart (Medium)
- On session restart (`agent_ready` after the session previously had a thread), Helix forces a full bidirectional state reconciliation rather than just a catch-up snapshot

### Issues #6, #8, #9 — Already addressed or no-code-fix
- Issue #6: `RecoverStaleBuilds` retry is in PR #1947 (already done)
- Issue #8: Docker kill contention — investigation only, no code fix for now
- Issue #9: Resolved by fixing #2 and #10

## Out of Scope
- ZFS pool topology changes
- Frontend redesign beyond the restart button
- PR #1947 contents (already implemented, pending merge)
