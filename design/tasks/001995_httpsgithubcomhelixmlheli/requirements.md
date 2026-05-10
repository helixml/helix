# Requirements — Fix `POST /sessions/{id}/messages` cold-start wake (issue #2397)

Issue: https://github.com/helixml/helix/issues/2397

## Problem

`POST /api/v1/sessions/{id}/messages` (added in https://github.com/helixml/helix/pull/2375) persists a `Waiting` interaction durably, but on a session whose Zed agent has never WebSocket-connected, the interaction sits in `waiting` forever. Nothing in the request path provokes the desktop / Zed agent to dial home, so `pickupWaitingInteraction` never has a peer to deliver to.

A real human visiting the desktop URL is currently the only thing that wakes the WebSocket. Once it's connected, queued messages flush immediately. External callers (helix-org, design-review, anything driving sessions programmatically without a human in the loop) cannot rely on `/messages` for first contact.

## Root cause (verified against the codebase, 2026-05-10)

`sendMessageToSession` (`api/pkg/server/spec_task_design_review_handlers.go:1497`) → `sendChatMessageToExternalAgent` (`api/pkg/server/websocket_external_agent_sync.go:1844`) → `sendCommandToExternalAgent` (`api/pkg/server/websocket_external_agent_sync.go:1910`) **already** fires `go apiServer.autoStartDevContainerForSession(sessionID)` (line 1925) when no WS exists.

The bug is one level deeper: `autoStartDevContainerForSession` (`api/pkg/server/websocket_external_agent_sync.go:3222`) only handles spec-task sessions. For any session where `session.Metadata.SpecTaskID == ""` it logs a debug line and returns:

```go
if session.Metadata.SpecTaskID == "" {
    log.Debug().Str("session_id", sessionID).Msg("autoStartDevContainerForSession: no spec task ID on session, skipping")
    return
}
```

The reporter's reproducer creates an *exploratory* `zed_external` session via `/api/v1/sessions/chat` with a `project_id` but no spec task. So:

1. `/sessions/chat` step starts the dev container (the container is `running` per the reproducer), but Zed inside hasn't dialled WebSocket yet.
2. `/sessions/{id}/messages` persists a `Waiting` interaction, calls `sendCommandToExternalAgent`, which fires `autoStartDevContainerForSession` — which silently no-ops because there's no spec task ID.
3. Nothing else in the codebase pokes Zed to connect. The interaction sits until a human opens the desktop URL.

The reporter's proposed Option A ("have sendMessageToSession call autoStartDevContainerForSession") is therefore necessary-but-not-sufficient — that wiring already exists; the silent no-op for non-spec-task sessions is the real defect.

## User stories

### As an external API caller (helix-org, automation)
- I want `POST /api/v1/sessions/{id}/messages` to wake a sleeping Zed agent without a human opening the desktop URL.
- I want this to work for both spec-task sessions AND exploratory `zed_external` sessions that have a `project_id` but no spec task.
- I want the contract documented in the OpenAPI spec to be true: "queued, will deliver on reconnect" must mean a reconnect is actually triggered.

### As an operator
- I want a defensive sweep that catches sessions stuck in `waiting` with no live WS, in case future code paths introduce the same bug.
- I want logs that make the cold-start wake path visible (start attempt, success, failure).

## Acceptance criteria

1. **Cold-start wake works for exploratory sessions.** A fresh `zed_external` session with a `project_id` (no spec task) created via `/sessions/chat` followed by `POST /sessions/{id}/messages` results in Zed dialling home and the queued message being delivered, without any human action. Reproducer in the issue body must pass end-to-end against the inner Helix.
2. **Cold-start wake still works for spec-task sessions.** Existing behaviour for sessions with `SpecTaskID != ""` is preserved (no regression).
3. **`autoStartDevContainerForSession` handles both shapes.** It builds a `DesktopAgent` from session metadata for both code paths (spec-task and exploratory), mirroring what `resumeSession` already does for HTTP-driven resumes.
4. **No double-start on already-connected sessions.** When a WS is live, `sendCommandToExternalAgent` does not call `autoStartDevContainerForSession` (current behaviour), and message dispatch goes straight over the existing socket.
5. **Defensive sweep (option B).** `auto_wake_stuck_interactions.go` re-pokes any session that has stuck `Waiting` interactions AND no live WS by calling `autoStartDevContainerForSession`. Bounded by the existing `autoWakeMaxRetries` cap so we don't spin forever on a session that genuinely cannot start.
6. **Logging.** Every cold-start auto-wake emits an INFO log line with `session_id`, `agent_type`, `has_spec_task`, and the outcome (started, already-running, failed). Failures are ERROR-level with the underlying error.
7. **Tests.**
   - Unit test: `autoStartDevContainerForSession` on an exploratory `zed_external` session calls `StartDesktop` (mocked executor) with the correct `DesktopAgent` fields populated from session metadata.
   - Unit test: `autoStartDevContainerForSession` on a session with neither spec task nor project ID logs and returns without error (we can't start what we don't have config for).
   - Unit test: existing `SessionMessagesHandlerSuite.TestQueuesWhenNoWS` still passes — but extend it to assert that `StartDesktop` was invoked when no WS is present.
   - Unit test for the auto-wake sweep: a stuck waiting interaction on a session with no WS triggers `autoStartDevContainerForSession` exactly once per scan tick, and stops after `autoWakeMaxRetries`.
8. **No new public API surface.** This is a behaviour fix; the `/messages` request/response contract does not change.

## Out of scope

- The "Zed accepts a keystroke but never relays it" failure mode (called out as a known coverage gap in `auto_wake_stuck_interactions.go:96-104`).
- The off-by-one ACP buffering issue tracked separately (`design/2026-04-25-zed-claude-async-event-flush-…`).
- Any UX changes to the desktop URL or frontend warmup — the fix is server-side only.
