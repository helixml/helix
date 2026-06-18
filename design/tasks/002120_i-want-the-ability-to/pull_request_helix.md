# Add clear-session capability for ACP and internal agent backends

## Summary
Adds the ability to "clear" a Helix session — wipe its conversation history so it
starts fresh — while keeping the session itself (ID, name, project, owner, model,
metadata) intact. Works for both runtimes a session can use: the internal Go agent
and the headless-Zed external agent. A single compositional entry point performs the
shared DB clear (the source of truth) and then delegates the runtime-specific reset
to the right backend, so callers don't need to know which runtime a session uses.

## Changes
- **Store**: new `Store.ClearSessionInteractions(ctx, sessionID)` — a single atomic
  `DELETE ... WHERE session_id = ?` over interactions, leaving the session row intact
  and idempotent on an already-empty session. Mock regenerated.
- **Compositional API** (`api/pkg/server/session_clear.go`):
  - `SessionBackend` interface (`Clear`), with `internalAgentBackend` and
    `zedACPBackend` implementations and a small `externalAgentTransport` seam
    (`*HelixAPIServer` satisfies it; tests inject a fake).
  - `backendFor(session)` dispatch (Zed/external vs internal).
  - `ClearSession` coordinator: clear DB → delegate backend → `TouchSession` →
    return the updated session.
- **Zed reset**: cancels any in-flight turn, then resets `Metadata.ZedThreadID` to ""
  so the next `chat_message` opens a brand-new Zed thread (`acp_thread_id=nil`, the
  same path forks use). No new protocol command — the server cannot mint Zed-valid
  thread IDs, and `open_thread` only re-opens an existing thread.
- **Internal reset**: effectively a no-op — internal sessions are request-scoped and
  re-seed from the DB, so the shared DB clear is sufficient.
- **HTTP**: `POST /api/v1/sessions/{id}/clear` (`clearSessionHandler`), authorized
  with `authorizeUserToSession(..., ActionUpdate)` like the sibling mutating routes;
  returns the cleared session, 404 unknown, 403 unauthorized.

## Tests
- `api/pkg/store/store_interactions_test.go` — seed N, clear, assert zero remain +
  session preserved + idempotent.
- `api/pkg/server/session_clear_test.go` — backends, `backendFor` dispatch,
  coordinator (internal + Zed), handler (200/404/403).
- Verified end-to-end against a running Helix (real Postgres): 3 interactions → clear
  → 200, interactions 3→0, session row preserved; idempotent re-clear 200; 404; 403.

## Out of scope
- Frontend "Clear conversation" button (backend + API only).
- Deleting the session entirely (already exists: `DELETE /api/v1/sessions/{id}`).
