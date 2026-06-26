# Design: Handle Deleted Agents Gracefully in Projects and Sessions

## Root cause

An agent is a Helix **App**, referenced by:
- `project.DefaultHelixAppID` (`api/pkg/types/project.go:237`) — drives **new** sessions.
- `session.ParentApp` / spec task `HelixAppID` — binds an **existing** session to its
  agent (`getAgentNameForSession`, `api/pkg/server/zed_config_handlers.go:504`; switch
  flow in `session_switch_agent_handlers.go`).

`deleteApp` (`api/pkg/server/app_handlers.go:1191`) deletes the App but clears **neither**
reference. So:

- **Facet A:** New thread loads can't register the agent → Zed returns
  `Custom agent server `claude-acp` is not registered` (`zed/.../custom.rs:258`), wrapped
  as `agent connect failed: …` (`zed/.../thread_service.rs:1698`) and sent to Helix as a
  `thread_load_error`. `handleThreadLoadError`
  (`api/pkg/server/websocket_external_agent_sync.go:3504`) stores the raw string and
  classifies it as transient → `MarkPromptAsFailed` → **infinite auto-retry**.
- **Facet B:** An already-started session keeps a dangling `ParentApp`. Nothing tells the
  user until they send a message and it fails the same way.

## Approach

Keep all changes in the **Helix Go + React layers** (the fork's existing error string is a
sufficient signal — no Rust rebuild). Three independent pieces:

### Piece 1 — Facet A: classify & translate "agent not configured" (Go)

`api/pkg/server/websocket_external_agent_sync.go`:
- Add `agentNotConfiguredMarkers` + `isAgentNotConfiguredError(errMsg)` next to the
  existing `isAgentCrashError`. Marker: `"is not registered"` (covers all the
  `... is not registered` variants). Robust because Zed already retries the "not
  registered" race 10× before surfacing it (`thread_service.rs:1686`) — when Helix sees
  it, it is terminal.
- In `handleThreadLoadError`, check this class **first**; when matched:
  - Set the interaction `Error` to a stable, friendly message (a fixed marker the
    frontend detects), e.g. `AGENT_NOT_CONFIGURED: This project doesn't have an agent
    configured…`.
  - Mark the prompt terminal via `MarkPromptAsCrashed` (pins `next_retry_at` far future,
    stopping auto-retry) — but **do not** call `maybeAutoRestartCrashedAgent` (restart
    can't conjure a deleted agent).
- Leave the crash and transient branches untouched.

### Piece 2 — Facet B: detect a session with a missing agent (Go)

The frontend cannot reliably tell "deleted" from "not visible to me" by scanning its apps
list, so the **backend** computes the truth.

- Add a computed boolean to the session GET response (e.g. `agent_available` /
  `agent_missing`) set when serving `GetSession`: resolve the session's agent App
  (spec-task `HelixAppID` first, then `ParentApp`); if a reference is set but the App no
  longer exists → `agent_missing = true`. Empty references on a session that requires an
  external agent also count as missing.
- Keep it a derived/transient field (not persisted) computed in the session handler that
  already serializes sessions, so it stays correct as apps come and go.

### Piece 3 — Facet B: proactive block + reassign in the UI (React)

- Read the new `agent_missing` flag in the session/spec-task chat view
  (`SpecTaskDetailContent.tsx`, which already renders `SwitchAgentControl` at line ~2001;
  and the generic session view `Session.tsx`).
- When `agent_missing` is true:
  - Render a prominent banner above the input:
    *"There is currently no agent assigned to this session. Before we can proceed, please
    assign one."*
  - **Disable the message input and send button** so a doomed message can't be sent.
  - Surface the agent picker inline — **reuse the existing `SwitchAgentControl` /
    `useSwitchAgent`** (`POST /api/v1/sessions/{id}/switch-agent`), which already repoints
    a session's agent in place and continues the conversation. Picking an agent here is a
    normal switch (the old, deleted app ≠ the new app, so the no-op guard passes).
  - After a successful switch, invalidate the session query; `agent_missing` flips false,
    the banner clears, and input re-enables.

### Piece 4 — Facet E: clear dangling references on delete (Go)

`deleteApp`: find projects with `DefaultHelixAppID == id` and clear the field. Simple
list + update loop with existing store methods. Stops new broken projects appearing;
existing breakage is handled by Pieces 1–3.

## Key Decisions

- **Translate in Helix, not Zed.** UX/classification belongs in the product layer; the
  fork only needs to keep emitting a recognizable string. Avoids a Rust rebuild +
  `sandbox-versions.txt` bump.
- **Backend computes "agent missing".** Authoritative; the frontend apps list can't
  distinguish deleted from hidden. A derived boolean on the session response is the
  smallest robust signal.
- **Reuse switch-agent for reassignment.** The in-place switch flow already exists, tested
  (`session_switch_agent_handlers_test.go`) and preserves the session/container/workspace.
  No new reassignment API is needed.
- **Prevention + cure.** Clearing `DefaultHelixAppID` on delete (Piece 4) stops the bug
  recurring; Pieces 1–3 recover already-stranded projects and sessions.

## Edge cases

- **Paused session:** `switchAgent` rejects paused sessions (switch on the active
  descendant). The banner should reflect this (assign on the live descendant) rather than
  offering a switch that 409s.
- **Non-external sessions:** the missing-agent block only applies to `zed_external`
  sessions (the only ones that bind to a switchable agent).

## Testing

- **Go unit tests** (`websocket_external_agent_sync_test.go`, existing gomock suite):
  `thread_load_error` with `"is not registered"` → friendly terminal message, not
  auto-retried; crash & transient cases unchanged.
- **Go test:** `GetSession` sets `agent_missing` when the bound App is deleted; false when
  it exists.
- **Go test:** deleting an App referenced by a project clears `DefaultHelixAppID`.
- **End-to-end (inner Helix, live Zed session — mandatory for lifecycle changes):**
  1. Create project + agent, start a live spec-task session, delete the agent App.
  2. Reopen the session → banner shows, input disabled, no message sent.
  3. Assign a new agent via the inline picker → banner clears, send a message, conversation
     continues on the same session.
  4. Confirm the Facet A friendly message + CTA also appear (no raw error, no retry loop).

## Files to touch (estimate)

- `api/pkg/server/websocket_external_agent_sync.go` — Facet A classifier + handling.
- `api/pkg/server/app_handlers.go` — clear dangling project references on delete.
- Session GET handler (`api/pkg/server/session_handlers.go`) + `api/pkg/types/types.go` —
  computed `agent_missing` field.
- `frontend/src/components/tasks/SpecTaskDetailContent.tsx`, `frontend/src/pages/Session.tsx`,
  `frontend/src/components/common/RobustPromptInput.tsx` — banner, input block, CTA.
- Tests: `websocket_external_agent_sync_test.go`, session handler test, store delete test.
