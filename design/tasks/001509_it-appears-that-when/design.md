# Design: Fix Build Cache Progress Reporting Bugs

## Overview

Two fixes to the golden cache copy progress tracking in Hydra. The core issue is that progress is keyed by **project ID** when it should be keyed by **session ID** (or volume name), and there's a race where stale progress entries can be read before the new copy begins.

## Architecture Context

The progress reporting pipeline:

```
SetupGoldenCopy (golden.go)
  └─ onProgress callback every 2s (du -sb on dest dir)
       └─ setGoldenCopyProgress(projectID, copied, total) ← BUG: keyed by projectID
            └─ goldenCopyProgress map[string]*GoldenCopyProgress

API-side polling (hydra_executor.go StartDesktop):
  └─ ticker every 2s
       └─ GetGoldenCopyProgress(projectID) ← reads same map key for all sessions
            └─ updateSessionStatusMessage(sessionID, msg) ← writes per-session (correct)
```

## Key Files

| File | Role |
|------|------|
| `api/pkg/hydra/devcontainer.go` | `GoldenCopyProgress` struct, `goldenCopyProgress` map, `setGoldenCopyProgress`, `GetGoldenCopyProgress`, `buildMounts` (calls `SetupGoldenCopy`) |
| `api/pkg/hydra/golden.go` | `SetupGoldenCopy` — runs the copy + progress monitor goroutine |
| `api/pkg/hydra/server.go` | HTTP handler `handleGetGoldenCopyProgress` |
| `api/pkg/hydra/client.go` | `GetGoldenCopyProgress` — RevDial client method |
| `api/pkg/external-agent/hydra_executor.go` | `StartDesktop` — polls progress and writes per-session status messages |

## Fix 1: Key Progress by Session/Volume Instead of Project ID

**Change**: Re-key `goldenCopyProgress` by volume name (which is unique per session: `docker-data-{sessionID}`).

### Hydra side (devcontainer.go)

- Change `goldenCopyProgress map[string]*GoldenCopyProgress` — key is now volume name instead of project ID.
- `setGoldenCopyProgress(volumeName, copied, total, done)` — use volume name as key.
- `GetGoldenCopyProgress(volumeName)` — look up by volume name.
- In `buildMounts`: pass `volumeName` instead of `req.ProjectID` to `setGoldenCopyProgress`.

### Hydra API (server.go + client.go)

- Change the endpoint from `/golden-cache/{project_id}/copy-progress` to `/golden-cache/copy-progress/{volume_name}` (or add a `volume_name` query param — simpler).
- Update `handleGetGoldenCopyProgress` to look up by volume name.
- Update `RevDialClient.GetGoldenCopyProgress` to accept volume name.

### API side (hydra_executor.go)

- The progress polling goroutine in `StartDesktop` already knows the session ID. It needs to also know the volume name used by Hydra. The volume name follows the pattern `docker-data-{sessionID}` — this is set in the devcontainer config. Pass session ID and derive the volume name, or have the Hydra endpoint accept session ID directly.
- **Simplest approach**: Use session ID as the progress key throughout (the volume name is derived from it anyway). Change `setGoldenCopyProgress` to take session ID. The `buildMounts` function has access to the session ID via `req.SessionID` (check this — it's on the `CreateDevContainerRequest`).

### Decision: Use session ID as the key

The volume name is `docker-data-{sessionID}`, so using session ID directly is cleaner than passing volume names through the API boundary. This avoids leaking Hydra-internal naming conventions to the API server.

## Fix 2: Eliminate the 100% → 0% Race

**Root cause**: The initial `onProgress(0, goldenSize)` call in `SetupGoldenCopy` (golden.go line 253) writes to the map immediately, but when two sessions race, session B's API poller can read session A's final 100% entry before session A's `setGoldenCopyProgress(..., true)` (done=true) deletes it. After deletion, session B's own `onProgress(0, goldenSize)` appears as a drop from 100% to 0%.

**Fix**: With session-level keying (Fix 1), this race goes away — each session only ever reads its own progress entry. Session A's 100% and cleanup never appear in session B's progress.

**Additional hardening**: Remove the initial `onProgress(0, goldenSize)` call in `SetupGoldenCopy` (line 253). The first real progress report should come from the `du -sb` ticker after 2 seconds. The API-side poller already handles `progress == nil` gracefully (continues polling). This means the progress simply doesn't appear until there's real data — which is correct behavior.

## Data Flow After Fix

```
SetupGoldenCopy(projectID, volumeName, onProgress)
  └─ du -sb ticker every 2s
       └─ onProgress(copied, goldenSize)
            └─ setGoldenCopyProgress(sessionID, copied, total, false)

API poller in StartDesktop:
  └─ GetGoldenCopyProgress(ctx, sessionID)  ← now session-scoped
       └─ returns only THIS session's copy progress
            └─ updateSessionStatusMessage(sessionID, msg)
```

## Scope & Non-Goals

- **In scope**: Fix the two bugs (shared progress, 100%→0% race).
- **Out of scope**: Changing the `du -sb` polling mechanism, adding a progress bar to the frontend (currently just text), or changing the polling intervals.

## Risks

- The API endpoint path change is internal (Hydra ↔ API server via RevDial) — no external consumers.
- The `GoldenCopyProgress` struct gains a `SessionID` field (or loses `ProjectID`) — internal only.
- Need to thread session ID through `buildMounts` → `SetupGoldenCopy` callback → `setGoldenCopyProgress`. The session ID is already available on `CreateDevContainerRequest`.