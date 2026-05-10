# Design — Fix `POST /sessions/{id}/messages` cold-start wake

Issue: https://github.com/helixml/helix/issues/2397

## Summary

Generalise `autoStartDevContainerForSession` so it can wake any `zed_external` session (spec-task OR exploratory project session), not just spec-task sessions. Then add a defensive sweep in `auto_wake_stuck_interactions.go` so the same recovery fires for sessions that fall into the stuck state via any code path, not just `/messages`.

## Current architecture (relevant slice)

```
POST /api/v1/sessions/{id}/messages
  └─▶ sendSessionMessage                                  session_handlers.go:2268
       └─▶ sendMessageToSession                           spec_task_design_review_handlers.go:1497
            └─▶ sendChatMessageToExternalAgent            websocket_external_agent_sync.go:1844
                 │   ├─ creates Waiting interaction
                 └─▶ sendCommandToExternalAgent           websocket_external_agent_sync.go:1910
                      ├─ if WS connected:    send command → done
                      └─ if WS not connected:
                           ├─ go autoStartDevContainerForSession(sessionID)   ← line 1925
                           └─ return ErrNoExternalAgentWS                       ← line 1926

autoStartDevContainerForSession(sessionID)                websocket_external_agent_sync.go:3222
  ├─ if session.Metadata.SpecTaskID == "": return         ← THE BUG: silent no-op
  └─ startDevContainerForSpecTask(specTask)               spec_task_design_review_handlers.go:910
       └─▶ externalAgentExecutor.StartDesktop(agent)
```

`resumeSession` (`session_handlers.go:1876`) already does the right thing for *both* shapes — spec-task AND exploratory — by reading `session.Metadata.SpecTaskID`, falling back to `session.Metadata.ProjectID`, building the `DesktopAgent` accordingly, and calling `StartDesktop`. The fix is to factor that body out and reuse it.

## Proposed change

### 1. Extract a shared session-resume helper

Move the agent-construction + `StartDesktop` invocation out of `resumeSession` (and out of `startDevContainerForSpecTask`) into a single helper, e.g.:

```go
// startDevContainerForSession boots the dev container for any zed_external session,
// whether it belongs to a spec task or is an exploratory project session.
// Returns nil if there is nothing to start (no spec task AND no project ID) — the
// caller's persisted Waiting interaction will simply remain queued.
func (s *HelixAPIServer) startDevContainerForSession(ctx context.Context, session *types.Session) error
```

The helper covers the three shapes `resumeSession` already handles (`session_handlers.go:1937-1960`):

- spec-task session: load `SpecTask`, take `ProjectID` / `OrganizationID` from it.
- exploratory session with `session.Metadata.ProjectID`: take project + org from session metadata.
- legacy session with `session.ProjectID`: take project + org from the session row.

If none of those produce a project ID, log INFO and return nil (we genuinely cannot start without project context — surface this clearly in logs but do not error).

Both `resumeSession` and `startDevContainerForSpecTask` then become thin wrappers over the helper. This is the **root-cause fix** the CLAUDE.md rules call for: one resume code path, no fallbacks, no dead branches.

### 2. Rewire `autoStartDevContainerForSession`

Replace the spec-task-only body with a call to the shared helper:

```go
func (apiServer *HelixAPIServer) autoStartDevContainerForSession(sessionID string) {
    ctx := context.Background()
    session, err := apiServer.Controller.Options.Store.GetSession(ctx, sessionID)
    if err != nil || session == nil {
        log.Error().Err(err).Str("session_id", sessionID).Msg("autoStartDevContainerForSession: failed to get session")
        return
    }
    if session.Metadata.AgentType != "zed_external" {
        return // non-external sessions don't have a desktop to wake
    }
    if startErr := apiServer.startDevContainerForSession(ctx, session); startErr != nil {
        log.Error().Err(startErr).Str("session_id", sessionID).Msg("Auto-start dev container failed")
    }
}
```

This makes the existing call from `sendCommandToExternalAgent:1925` actually effective for the reproducer scenario.

### 3. Defensive sweep in `auto_wake_stuck_interactions.go`

Today the worker bails at Gate 1 (`auto_wake_stuck_interactions.go:237-240`) if no WebSocket exists. Change Gate 1 so that when there is no WS but the session is `zed_external` AND has stuck interactions, it fires `autoStartDevContainerForSession` (the same call `sendCommandToExternalAgent` makes) — bounded by the existing `AutoWakeCount` retry counter so a permanently-broken session doesn't churn forever.

Skeleton:

```go
conn, connected := apiServer.externalAgentWSManager.getConnection(stuck.SessionID)
if !connected || conn == nil {
    if stuck.AutoWakeCount >= autoWakeMaxRetries {
        // Already tried; mark error so we stop scanning.
        apiServer.markStuckInteractionAsError(ctx, stuck, "no WS after auto-wake retries")
        return
    }
    log.Info().
        Str("session_id", stuck.SessionID).
        Int("attempt", stuck.AutoWakeCount + 1).
        Msg("[AUTO_WAKE] No WS for stuck interaction — kicking dev container auto-start")
    apiServer.bumpAutoWakeCount(ctx, stuck) // increments the column so the cap engages
    go apiServer.autoStartDevContainerForSession(stuck.SessionID)
    return
}
// existing path: WS is live, re-send the prompt
```

The bump must use a targeted column update (same pattern the worker already uses for the wake-up retry), not a `Save` that could be clobbered by the streaming path (the existing comment at `auto_wake_stuck_interactions.go:75-86` explains why).

### 4. Idempotency on already-running containers

`StartDesktop` is the same call `resumeSession` makes when a user clicks "resume" — so its existing idempotency story applies. The reporter's reproducer ("container is `running` but Zed has not connected") confirms that opening the desktop URL (which triggers the same `StartDesktop` path via the frontend) wakes Zed. So calling `StartDesktop` on an already-running container DOES kick Zed to dial home — that's the established mechanism. We do not need a separate "already running, just nudge Zed" code path.

## Decisions and trade-offs

**Why generalise `autoStartDevContainerForSession` instead of just calling `resumeSession` from the message handler?** `resumeSession` is HTTP-bound (writes to `http.ResponseWriter`, reads `mux.Vars`, returns 4xx/5xx). The auto-wake call sites (`sendCommandToExternalAgent`, the worker) are not HTTP handlers — they need the pure business logic. Extracting the helper is the standard Go refactor and makes the duplicated bodies in `startDevContainerForSpecTask` and `resumeSession` collapse into one source of truth.

**Why also do option B?** Option A alone fixes the reported reproducer. Option B is a low-cost safety net for any future code path that creates a `Waiting` interaction without going through `sendCommandToExternalAgent` — and it costs only a handful of lines now that the helper exists. The reporter explicitly endorsed both fixes; doing both leaves no easy way to regress this.

**Why not just remove the spec-task gate without extracting a helper?** That would leave us with two near-identical 100-line agent-building blocks (one in `resumeSession`, one in `startDevContainerForSpecTask`) that drift over time. The helper extraction is required to avoid that drift.

**Retry budget for the sweep.** Reuse `autoWakeMaxRetries = 2` (already declared in `auto_wake_stuck_interactions.go:164`). After two failed cold-start attempts on a session with no WS, mark the interaction `state=error` so subsequent scans skip it — same termination policy the existing wake-up uses.

**No frontend changes.** The fix is server-side only. The frontend keeps doing what it does today (the desktop URL still triggers a resume; nothing breaks).

## Files touched

| File | Change |
|---|---|
| `api/pkg/server/spec_task_design_review_handlers.go` | New `startDevContainerForSession(ctx, session)` helper. Existing `startDevContainerForSpecTask` reduces to a thin wrapper. |
| `api/pkg/server/session_handlers.go` | `resumeSession` body refactored to call the new helper for the agent-build + StartDesktop section. HTTP-specific code (auth, response writing, metadata refresh) stays in the handler. |
| `api/pkg/server/websocket_external_agent_sync.go` | `autoStartDevContainerForSession` rewritten to call the new helper for any `zed_external` session. |
| `api/pkg/server/auto_wake_stuck_interactions.go` | Gate 1 extended: when no WS, kick `autoStartDevContainerForSession` once per cap-bounded attempt instead of returning silently. |
| `api/pkg/server/session_messages_handler_test.go` | Extend `TestQueuesWhenNoWS` to assert `StartDesktop` invocation via mock. |
| `api/pkg/server/auto_wake_stuck_interactions_test.go` (or the existing test file) | New tests for the no-WS sweep path. |
| `api/pkg/server/spec_task_design_review_handlers_test.go` (or new) | Unit tests for the new helper across the three session shapes. |

## Verification plan

1. Local Go build of `api/pkg/server/`: `cd api && CGO_ENABLED=1 go test -run TestSessionMessagesHandlerSuite ./pkg/server/ -count=1`.
2. End-to-end against the inner Helix at `http://localhost:8080`:
   - Register `test@helix.ml` / `helixtest`, create org/project, get an API key.
   - Run the issue's reproducer (POST /sessions/chat to create the session, then POST /sessions/{id}/messages).
   - Watch `docker compose -f docker-compose.dev.yaml logs -f api` for the auto-start log line.
   - Confirm the queued message gets a response within ~30s without opening the desktop URL.
3. Regression: existing spec-task auto-start path still works (run an existing spec task and confirm no behaviour change).
4. Push and check CI (Drone).

## Notes for future-me / cloned tasks

- The CLAUDE.md rules in this repo explicitly forbid adding hacks that paper over root causes ("If you find yourself adding hacks or workarounds, **stop**"). The reporter's option A as stated would be a hack — adding a redundant call in `sendMessageToSession` without addressing why the existing call from `sendCommandToExternalAgent` is silently failing. The right move is the helper extraction.
- `resumeSession` and `startDevContainerForSpecTask` were already drifting (compare display-settings handling, error wrapping, metadata refresh details). The helper extraction also pays down that debt.
- If you find yourself wanting to add a "already running, just nudge" branch — first verify it's needed. The reporter's empirical observation is that `StartDesktop` (via the desktop URL) DOES wake Zed even when the container is already running. Trust that data point until proven otherwise.
