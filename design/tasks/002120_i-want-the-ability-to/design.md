# Design: Add Clear Session Capability for ACP and Internal Agent Backends

## Problem & Approach

Today a Helix `Session` (`api/pkg/types/types.go:747`) is "one massive thing":
there is no service-layer abstraction over session operations, and no way to clear
its conversation. Interactions live in their own DB table; the store exposes
`DeleteInteraction(ctx, id)` (`api/pkg/store/store_interactions.go:302`) for a
single interaction, but no batch clear.

Two runtimes drive a session and each holds extra state:
- **ACP / Zed** — `api/pkg/server/websocket_external_agent_sync.go`. Commands are
  JSON `types.ExternalAgentCommand{Type, Data}` sent over WS. Existing types:
  `chat_message`, `open_thread`, `cancel_current_turn`. Zed keeps its own thread
  context (`SessionMetadata.ZedThreadID`).
- **Internal Go agent** — `api/pkg/agent/session.go`, history in a `MessageList`
  which already has a `Clear()` method (`api/pkg/agent/llm_message.go:87`). The
  in-memory session is ephemeral and re-seeds its history from DB interactions.

**Key insight:** the **DB interactions are the source of truth** for both backends.
The internal agent re-seeds from the DB on the next turn, so clearing the DB is
sufficient for it. The Zed agent keeps an independent context, so it *additionally*
needs to be told to start a fresh thread. This asymmetry is exactly what the small
compositional API absorbs.

## The Compositional API

Introduce a minimal backend interface and a single coordinator method. This is the
"nice compositional API for a Helix session" the request asks for, scoped to just
what `Clear` needs (we do not refactor the whole session model).

```go
// api/pkg/session (new small package) or api/pkg/controller

// SessionBackend is the per-runtime half of a session operation.
type SessionBackend interface {
    // Clear resets the runtime-specific conversational state for a session.
    // DB interactions are cleared by the coordinator, not here.
    Clear(ctx context.Context, sessionID string) error
}
```

Two implementations:

- **`internalAgentBackend`** — `Clear` is effectively a no-op against the DB-cleared
  state. If a live in-memory `agent.Session` exists for the session, call its
  `GetMessageHistory().Clear()`; otherwise nothing to do (next turn seeds empty).
- **`zedACPBackend`** — `Clear`:
  1. Cancels any in-flight turn (reuse `sendCancelToExternalAgent` /
     `cancel_current_turn`) so streaming can't repopulate history.
  2. Allocates a fresh `ZedThreadID`, persists it via `UpdateSessionMetadata`.
  3. Sends a clear/new-thread command to Zed. Prefer reusing the existing
     `open_thread` command with the new thread id (Zed already handles it on
     reconnect at `websocket_external_agent_sync.go:457`); if Zed needs an explicit
     "forget current thread" signal, add a `clear_thread` command type following the
     ack pattern of `cancel_current_turn` (request_id + `pendingCancelChannels`).
  4. If there is no live connection (`ErrNoExternalAgentWS`), treat as success — the
     new `ZedThreadID` is persisted and applied when the agent next connects.

Coordinator (the method that "hangs off the session"):

```go
func (c *Controller) ClearSession(ctx context.Context, sessionID string) (*types.Session, error) {
    session := c.store.GetSession(...)           // load + authorize upstream
    if err := c.store.ClearSessionInteractions(ctx, sessionID); err != nil { ... }  // shared, source of truth
    backend := c.backendFor(session)             // dispatch on CodeAgentRuntime / ExternalAgentConfig
    if err := backend.Clear(ctx, sessionID); err != nil { ... }
    c.store.TouchSession(ctx, sessionID)
    return c.store.GetSession(ctx, sessionID)
}
```

Backend selection (`backendFor`) mirrors existing dispatch: a session is Zed/ACP
when `Metadata.CodeAgentRuntime == zed_agent` (or `ExternalAgentConfig`/
`ExternalAgentID` is set); otherwise internal agent.

## Store Layer

Add one method to the `Store` interface (`api/pkg/store/store.go`) and its Postgres
implementation:

```go
ClearSessionInteractions(ctx context.Context, sessionID string) error
```

Implemented as a single delete:
`db.Where("session_id = ?", sessionID).Delete(&types.Interaction{})`.
This is a hard delete of conversation rows (interactions are not soft-deleted today;
`DeleteInteraction` is a hard delete). The session row is untouched.

## HTTP Layer

New route in `api/pkg/server/server.go` (sessions block ~line 1023), next to the
existing `cancel` / `restart-agent` routes:

```
POST /api/v1/sessions/{id}/clear  -> clearSessionHandler
```

Handler in `api/pkg/server/session_handlers.go` follows the existing pattern
(`deleteSession` at line 246): load session, authorize with the same helper used by
sibling handlers, call `controller.ClearSession`, return the session via the
`system.Wrapper` JSON/error convention.

## Streaming-safety note

The Zed backend writes streamed tokens through `streamingContext`
(`websocket_external_agent_sync.go:73`) with throttled DB writes. Clearing **must**
cancel the active turn first (step 1 above) so an in-flight `handleMessageCompleted`
doesn't re-insert an interaction after the delete. The internal agent is request-
scoped, so clearing between turns is safe; clearing mid-turn is out of scope (UI
should disable clear while a turn is running, consistent with cancel).

## Key Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Clear vs delete session | Clear keeps the session row, removes interactions only | Matches user intent ("start fresh in same session"); delete already exists |
| Source of truth | DB interactions | Internal agent re-seeds from DB; one shared delete covers both backends |
| Compositional shape | Small `SessionBackend{ Clear }` interface + coordinator | Minimal abstraction that satisfies "method hangs off the session" without refactoring the whole model |
| Zed reset mechanism | Reuse `open_thread` with new `ZedThreadID`; add `clear_thread` only if Zed needs explicit signal | Reuses proven path; avoids new protocol surface unless required |
| Batch delete | New `ClearSessionInteractions` raw delete | Atomic, avoids per-row loop over `DeleteInteraction` |
| No-connection case | Persist new thread id, succeed | Transient disconnects are normal during boot (`ErrNoExternalAgentWS`) |

## TDD Plan (tests first)

1. **Store**: `ClearSessionInteractions` — seed a session with N interactions, clear,
   assert zero interactions remain and the session row still exists. Follow existing
   store test setup in `api/pkg/store/store_interactions_test.go`.
2. **Internal agent backend**: `MessageList.Clear()` already covered; add a backend
   test asserting `Clear` empties a live history and no-ops when none exists.
3. **Zed backend**: with a mocked WS sender + store, assert: cancel sent, new
   `ZedThreadID` persisted, `open_thread`/`clear_thread` command sent; and the
   no-connection path returns success.
4. **Coordinator**: `ClearSession` dispatches to the correct backend based on
   `CodeAgentRuntime` and always calls `ClearSessionInteractions`.
5. **Handler**: `POST /sessions/{id}/clear` returns 200 on success, 404 unknown,
   403 unauthorized — using existing handler test harness.

## Files Touched (implementation phase)

- `api/pkg/store/store.go` — interface method
- `api/pkg/store/store_interactions.go` — `ClearSessionInteractions` impl
- `api/pkg/store/store_interactions_test.go` — store test
- `api/pkg/controller/` (or new `api/pkg/session/`) — `SessionBackend`, two impls, `ClearSession`
- `api/pkg/server/session_handlers.go` — `clearSessionHandler`
- `api/pkg/server/server.go` — route registration
- `api/pkg/server/websocket_external_agent_sync.go` — `clear_thread` command (only if needed)
- Regenerate store mocks if a mock of the `Store` interface exists.
