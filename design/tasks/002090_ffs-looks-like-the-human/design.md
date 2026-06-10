# Design: Worker Activation Must Reuse the Project's Human Desktop Session

## TDD-first ordering

Write the failing test before touching the production code. The whole
point of this task is to land a regression gate; the production fix is
the smaller half of the work. Sequence:

1. Add the test (Section "Test Plan" below). Confirm it fails on `main`
   with the expected duplicate-row message.
2. Make the minimal production change (Section "Production Change") to
   turn it green.
3. Re-run the entire `./api/pkg/server/...` and
   `./api/pkg/org/infrastructure/runtime/helix/...` suites — flush out
   anything that was incidentally relying on duplicate-row behaviour.

## Summary

`StartExternalAgentSession` is the single primitive every "create a
session + start its desktop" path funnels through. Today it
unconditionally mints `system.GenerateSessionID()` and writes a fresh
session row. Make it conditionally reuse the project's existing
exploratory session row when the request is itself an exploratory
session request (`req.SessionRole == "exploratory"` and
`req.ProjectID != ""`).

That single guard fixes both regression paths simultaneously:

- The helix-org spawner's `inProcHelixClient.StartSession` already sets
  `SessionRole: "exploratory"` and a `ProjectID`
  (`helix_org_inproc.go:478-485`), so it reuses the row created by
  `startExploratorySession`.
- `startExploratorySession` itself does its own lookup before deciding
  to create (`project_handlers.go:1348`), so the new guard in
  `StartExternalAgentSession` is invisible from that path.

After the guard: at most one exploratory row per project ever exists,
which is what `GetProjectExploratorySession`'s `LIMIT 1` was always
implicitly assuming.

## Affected Files

| File | Change |
|---|---|
| `api/pkg/server/session_handlers.go` | In `StartExternalAgentSession` (line 2421), before generating a session ID, look up an existing exploratory row for `req.ProjectID` when `req.SessionRole == "exploratory"`. If found, reuse it: assign `session.ID = existing.ID`, persist the new prompt/interaction against the reused row, and call `StartDesktop` with the reused id (idempotent at the hydra layer — same `agent.SessionID` collapses to the "already running" fast-path). |
| `api/pkg/server/exploratory_session_activation_test.go` | **New file.** Suite-based test (mirrors the `StartDevContainerForSessionSuite` style at `start_dev_container_test.go`). Cases listed under "Test Plan". |
| `api/pkg/org/infrastructure/runtime/helix/spawner.go` | (Optional defensive belt-and-braces.) Before calling `EnsureAndSend` with an empty `state.SessionID`, query the project's exploratory session via `c.Client.ExploratorySession` and pre-fill `SendPromptParams.SessionID` from it. This collapses worker activations onto the existing row even if the server-side guard is bypassed in tests. Keep behind a one-line `if c.Client.ExploratorySession != nil` nil-guard. |

No DB migrations. No frontend changes. The single guard inside
`StartExternalAgentSession` is the load-bearing piece; the spawner
change is an optional defence in depth.

## Production Change (the load-bearing 10 lines)

`StartExternalAgentSession` at `session_handlers.go:2421`. Today:

```go
session := &types.Session{
    ID:             system.GenerateSessionID(),
    Name:           s.getTemporarySessionName(message),
    ...
    ProjectID:      req.ProjectID,
    ...
    Metadata: types.SessionMetadata{
        ...
        ProjectID:    req.ProjectID,
        SessionRole:  req.SessionRole,
        ...
    },
}
```

After:

```go
// Exploratory sessions are project-scoped singletons — at most one
// session per (project_id, session_role="exploratory") so the UI's
// GetProjectExploratorySession lookup (LIMIT 1) can't disagree with
// whichever row StartDesktop registered against the hydra map.
// Reuse the existing row instead of minting a parallel one.
sessionID := ""
if req.SessionRole == "exploratory" && req.ProjectID != "" {
    if existing, err := s.Store.GetProjectExploratorySession(ctx, req.ProjectID); err == nil && existing != nil {
        sessionID = existing.ID
    }
}
if sessionID == "" {
    sessionID = system.GenerateSessionID()
}

session := &types.Session{
    ID:             sessionID,
    Name:           s.getTemporarySessionName(message),
    ...
}
```

Then, downstream of the existing `session` build:

- If `sessionID` was reused, the new prompt/interactions append to the
  existing row instead of writing a fresh `WriteSession`. Concretely:
  branch on `sessionID == existing.ID` and call `UpdateSession` +
  `WriteInteractions` instead of `WriteSession` + `WriteInteractions`,
  preserving the existing session's `Owner`, `OrganizationID`,
  `Metadata` fields that the caller didn't supply.
- The `StartDesktop` call is unchanged — it's already keyed by
  `agent.SessionID`, and the hydra fast-path at
  `hydra_executor.go:149` ("Dev container already running, returning
  existing session") makes a same-id second call a no-op.

### Why not "delete the old row and create a new one"

Tempting because it sidesteps the
`WriteSession`-vs-`UpdateSession` branching. Rejected:

1. The session id is what the running desktop container is keyed by
   (`h.sessions[agent.SessionID]`) and what every WebSocket client has
   subscribed to. Deleting + re-creating with a different id silently
   disconnects every observer.
2. Interactions are foreign-keyed to `session_id`. A delete cascades
   conversation history; we'd lose the operator's previous prompts
   from the Resume case.
3. The reuse path mirrors how `startExploratorySession` already
   behaves when it finds an existing row (`project_handlers.go:1349-1498`
   — restart container, keep row id). Convergent behaviour, not
   parallel.

## Spawner-side belt-and-braces (optional, lower priority)

`SpawnerConfig.ensureSession` at `spawner.go:430` currently does:

```go
sid, fresh, err := EnsureAndSend(ctx, c.Client, SendPromptParams{
    SessionID: state.SessionID,
    ProjectID: state.ProjectID,
    ...
})
```

If `state.SessionID == ""`, `EnsureAndSend` calls `StartSession` which
mints. The server-side guard catches this — but if you want the
spawner to never even ask the server to mint, pre-fill `SessionID`
from the project's exploratory row:

```go
sessionID := state.SessionID
if sessionID == "" && c.Client.ExploratorySession != nil {
    if sid, err := c.Client.ExploratorySession(ctx, state.ProjectID); err == nil && sid != "" {
        sessionID = sid
    }
}
sid, fresh, err := EnsureAndSend(ctx, c.Client, SendPromptParams{
    SessionID: sessionID,
    ...
})
```

`ExploratorySession` is already declared on `SpawnerClient` (see
`mirror.go:26-29`); it's the same lookup `getProjectExploratorySession`
uses. Wire it through `SpawnerConfig` if it isn't already.

This is **defence in depth**. The server-side guard is the actual
fix; this change exists so a future caller that bypasses
`StartExternalAgentSession` (a direct `s.Store.CreateSession` call from
a new code path, say) still converges on the singleton. Land it in the
same PR if cheap; defer to a follow-up if it grows tendrils.

## Why this is safe

1. **Idempotent at the hydra layer.** `StartDesktop` with a SessionID
   already in `h.sessions` and status `running` returns the existing
   session record (`hydra_executor.go:149-159`). Re-resume of a live
   container is a no-op.
2. **No-op at the row-creation layer.** Reusing the row preserves all
   metadata (owner, organization_id, parent_app, agent_type, etc.).
   `UpdateSession` writes only the fields the new request actually
   touched — same pattern `startExploratorySession` already uses on its
   restart branch.
3. **Per-project singleton was always the de facto contract.**
   `GetProjectExploratorySession` has been `ORDER BY created DESC LIMIT 1`
   since 1c8ad6fc7 (2026-04-24). Every caller that reads "the"
   exploratory session has been silently assuming there's only one;
   we're closing the gap where there could be two.
4. **No new auth surface.** The guard reuses the row the caller had
   permission to read; the existing `authorizeUserToProject` and
   `authorizeUserToSession` checks elsewhere already gate access.

## Test Plan

### New file: `api/pkg/server/exploratory_session_activation_test.go`

Build on the existing test infrastructure (`test_helpers.go`'s
`NewTestServer` + `memorystore`). The suite needs a `mockExecutor`
because we're asserting against StartDesktop calls; reuse the
`MockExecutor` GoMock from `executor_mocks.go`.

```
type ExploratorySessionActivationSuite struct {
    suite.Suite
    server   *HelixAPIServer
    ctx      context.Context
    user     *types.User
    project  *types.Project
    mockExec *external_agent.MockExecutor
}

func TestExploratorySessionActivationSuite(t *testing.T) {
    suite.Run(t, new(ExploratorySessionActivationSuite))
}
```

#### Case 1: regression gate — single exploratory row across resume + activation

```
1. Pre-seed: project P (org O, owner U). No sessions yet.
2. Call startExploratorySession(P) via the handler.
   Capture returned session row A.
3. Assert: SELECT COUNT(*) FROM exploratory rows for P == 1.
4. Call StartExternalAgentSession with:
     req.ProjectID    = P.ID
     req.SessionRole  = "exploratory"
     req.AgentType    = "zed_external"
     (one message: "ping")
   This is what helix-org's inProcHelixClient.StartSession sends.
5. Assert (the regression gate): SELECT COUNT(*) FROM exploratory rows
   for P == 1 (NOT 2). Fails on main with
   "expected 1 exploratory row for project P, got 2".
6. Assert: returned session.ID == A.ID.
7. Assert: mockExec.StartDesktop was called with agent.SessionID == A.ID
   on both invocations (first from startExploratorySession, second from
   StartExternalAgentSession). Two calls total, identical SessionID.
```

#### Case 2: status pill flips to "running" after Resume

```
1. Pre-seed exploratory session A for project P (Metadata.ProjectID=P,
   SessionRole="exploratory"). mockExec configured so the first
   GetSession(A) returns ErrNotFound (cold state).
2. GET /projects/P/exploratory-session → expect
   config.external_agent_status == "stopped".
3. POST /sessions/A/resume → resumeSessionInternal runs → StartDesktop
   called with SessionID=A. Configure mockExec.GetSession(A) to now
   return a running ZedSession.
4. GET /projects/P/exploratory-session → expect
   config.external_agent_status == "running".
```

#### Case 3: no project → no reuse (negative path)

`StartExternalAgentSession` with `req.SessionRole=="exploratory"` and
`req.ProjectID==""` must still mint a fresh id. The guard is gated on
`req.ProjectID != ""`. Confirms we didn't accidentally make every
exploratory session globally singleton.

#### Case 4: different `session_role` → no reuse (negative path)

`req.SessionRole=="planning"` with the same `req.ProjectID` as case 1.
Must mint a fresh id — `GetProjectExploratorySession`'s WHERE clause
hard-codes `session_role='exploratory'` so we can't accidentally
collide planning sessions.

### Existing tests that must keep passing

- `start_dev_container_test.go:86` `TestExploratoryShape` — pins
  session shape for the resume path. Unaffected.
- `auto_wake_stuck_interactions_test.go` — exercises
  `ExternalAgentStatus` transitions on a single session row.
  Unaffected.
- `session_etag_test.go` — ETag flips on `ExternalAgentStatus` change.
  Unaffected.
- Whatever spawner tests exist in
  `api/pkg/org/infrastructure/runtime/helix/spawner_test.go` — these
  use `fakeHelixClient` whose `StartSession` returns
  `f.startSessionID`. The fake doesn't go through the real DB lookup;
  tests stay green. Add one new fake case if you implement the
  spawner-side belt-and-braces: `ExploratorySession` returning a
  pre-seeded id, then `EnsureAndSend` must call `SendMessage` (not
  `StartSession`) because the spawner pre-filled `SessionID`.

### Manual end-to-end (see requirements.md §C)

Mirrors the user's exact repro. Run it after the unit tests pass so
the green CI doesn't lull anyone into skipping the real-stack check.

## Files Read During Investigation (for future implementers)

These are the call sites and code paths that matter — skim them before
implementing, in this order:

1. `frontend/src/pages/SpecTasksPage.tsx:948-1010` — the three-state
   button (Open / Resume / View).
2. `frontend/src/services/projectService.ts:280-339` — the
   start/stop/resume mutation hooks. Note that Resume does GET → POST
   `/sessions/{id}/resume`, not a project-scoped endpoint.
3. `api/pkg/server/project_handlers.go:1253-1644` — the three exploratory
   handlers (`get`, `start`, `stop`). `getProjectExploratorySession` is
   where the status pill data comes from.
4. `api/pkg/server/session_handlers.go:1886-2117` — `resumeSession`
   handler and the `resumeSessionInternal` helper.
5. `api/pkg/server/session_handlers.go:2421-2529` — `StartExternalAgentSession`,
   the load-bearing primitive.
6. `api/pkg/store/store_sessions.go:309-330` — `GetProjectExploratorySession`,
   the `ORDER BY created DESC LIMIT 1` query.
7. `api/pkg/external-agent/hydra_executor.go:130-159, 666-679` —
   `StartDesktop` registration and `GetSession` lookup against
   `h.sessions[SessionID]`.
8. `api/pkg/org/infrastructure/runtime/helix/spawner.go:430-470` —
   `ensureSession` and how it threads `state.SessionID` into
   `EnsureAndSend`.
9. `api/pkg/org/infrastructure/runtime/helix/sessions.go:23-150` —
   `EnsureAndSend` semantics. Specifically: `StartSession` is what runs
   when `state.SessionID==""`.
10. `api/pkg/server/helix_org_inproc.go:466-496` — the adapter that
    sends `SessionRole: "exploratory"` and `ProjectID:`-set requests
    into `StartExternalAgentSession`. This is *why* the server-side
    guard works: every exploratory creation request the spawner makes
    already carries the (project_id, role=exploratory) tuple the guard
    keys on.

## Risk

Low — load-bearing change is a 10-line guard with one new code path
(reuse vs mint), the reused side is the path `startExploratorySession`
has been using since the feature shipped, and the failure mode the
guard prevents is well-described by the regression test. The only
non-obvious sharp edge is "interaction history merges across
activations" — this is the *desired* behaviour (the desktop is a
long-lived session, not per-activation), and the existing comment
above `inProcHelixClient.StartSession`
(`helix_org_inproc.go:466-469`) already documents that as the
intended shape.
