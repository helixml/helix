# Design: Fix Restart Agent Session to Fully Reset Desktop and Context

## Goal

The Bot Detail page "Restart agent session" button must perform a **full
restart**: destroy the current session + desktop + workspace and create a
brand-new session (new session ID) with a fresh desktop and empty context, then
show that new session in the chat window. This is distinct from the crash-
recovery restart used elsewhere, which intentionally preserves context.

## Current flow (broken)

```
HelixOrgBotDetail.tsx  "Restart agent session"
  → useRestartBotAgent  (helixOrgService.ts)
  → POST /api/v1/orgs/{org}/bots/{id}/restart-agent
  → restartBotAgent          (api/pkg/org/interfaces/server/api/bots.go:466)
      resolves sessionID via BotRuntime.State
  → SessionRestarter.RestartSession(sessionID)
  → restartSessionContainer  (api/pkg/server/session_handlers.go:2451)
        StopDesktop (best-effort, errors swallowed)
        resumeSessionInternal  ← PRESERVES ZedThreadID + workspace volume + session ID
        ResetCrashedPromptsForSession
```

`restartSessionContainer` is a context-preserving crash-recovery primitive.
Reusing it for the bot-page button is the defect — it never destroys the session
or forces a truly new desktop.

## How sessions work (relevant mechanics)

- **Exploratory session = project singleton.** One row per
  `(project_id, session_role="exploratory")`. `StartExternalAgentSession`
  (`session_handlers.go:2669`) **reuses** an existing exploratory row if present
  (deliberate guard against parallel sessions), else mints a new one +
  `StartDesktop`.
- **"Current session" resolution.** The mirror (`mirror.go:resolveSession`)
  prefers `GetProjectExploratorySession` (`store_sessions.go`, `ORDER BY created
  DESC`) over the persisted `BotRuntimeState.SessionID` pointer.
- **Runtime state pointer.** `SaveSession` (`state.go:88`) persists the bot's
  session ID under `(orgID, workerID, backend="helix", key="session_id")`.
- **Existing high-level ops.** Helix already exposes delete-session
  (`DELETE /sessions/{id}` → `deleteSession`) and start-session
  (`StartSession` → `StartExternalAgentSession`). Deleting a session tears down
  its desktop; starting a new one brings up a fresh desktop. The full-restart
  flow just composes these — no need to touch container/workspace internals.

## Proposed change

Compose two **existing high-level Helix operations** — delete a session, then
start a new one. We must NOT reach into container/workspace internals (ZFS,
`DeleteDevContainer`, volume deletes); Helix already deletes sessions and creates
new ones cleanly, and deleting a session tears down its desktop.

Give `restartBotAgent` a dedicated **full-restart** backend flow (a new port on
the org api adapter, e.g. `BotFullRestarter`/`FullRestart(ctx, orgID, botID)`),
implemented in the in-proc helix client alongside the existing
`StopExternalAgent` / `StartSession` wrappers. Do **not** reuse
`restartSessionContainer`. Algorithm:

1. **Resolve the bot's current session** from `BotRuntime.State` /
   exploratory lookup. If none exists, skip to step 3 (first-time start).
2. **Delete the old session** via Helix's existing delete-session operation —
   `DELETE /api/v1/sessions/{id}` (`deleteSession`, `session_handlers.go:246`).
   Add a thin in-proc wrapper that calls this handler, exactly mirroring the
   existing `StopExternalAgent` wrapper (`helix_org_inproc.go:456`, which wraps
   `stopExternalAgentSession`). Deleting the session stops its desktop and
   removes it as the exploratory singleton, so the next start mints a new one
   instead of reusing it. Surface failures (don't swallow).
3. **Create a new session** on the same project via the existing `StartSession`
   primitive (`helix_org_inproc.go:470` → `StartExternalAgentSession`), the same
   one the spawner/cron use. It mints a **new session ID** and brings up a fresh
   desktop with fresh MCP services.
4. **Persist the new session ID** into runtime state via `SaveSession(ctx,
   store, orgID, botID, newSessionID)` so the mirror and future activations
   resolve the new session.
5. **Return the new session ID** in `BotActivateDTO.SessionID`.

First-time start (no existing session): step 2 is a no-op; step 3 provisions the
project (via `Activate`/ensurer) and starts the first session.

Reusing these two existing operations means the desktop teardown and fresh-
desktop provisioning are handled by Helix's own session lifecycle — this spec
does not touch hydra/ZFS internals.

### Why a new flow, not a `fresh` flag on restartSessionContainer

`restartSessionContainer` keeps the same session ID by design and is shared with
the in-chat / spec-task crash-recovery surfaces. A full restart needs a
different shape (destroy + mint-new + repoint runtime state), so it gets its own
flow. The crash-recovery primitive stays untouched (AC-7).

### Frontend — switch the chat window to the new session

The chat/desktop panels are bound to `chatSessionId` (`HelixOrgBotDetail.tsx:171`),
the bot's exploratory session, resolved once via
`fetchExistingWorkerSession(projectID, chatApi)` and rendered by
`EmbeddedSessionView`.

No confirmation dialog. In `handleRestartSession`, after
`restartAgent.mutateAsync` resolves:

- Read the **new** session id from the mutation response
  (`BotActivateDTO.SessionID`) and set `chatSessionId` to it. This re-binds the
  transcript, desktop stream, and WebSocket (`streaming.setCurrentSessionId`) to
  the new session, so the old transcript disappears and the fresh (empty) one
  shows.
- If the id needs a forced remount (e.g. `EmbeddedSessionView` caches by id),
  null then set `chatSessionId`, or fall back to re-running
  `fetchExistingWorkerSession(projectID)` which now resolves the new
  most-recent exploratory session.
- Default the panel to the Chat tab (`sessionTab='chat'`).
- Update the success snackbar to "fresh session started"; keep error handling
  (`err?.response?.data?.error`) so a failed backend response surfaces an error.
- Ensure the mutation hook (`useRestartBotAgent` in `helixOrgService.ts`) returns
  the DTO so the new `SessionID` is available to the handler.

## Key files

| Concern | File |
|---|---|
| Button + handler + chat re-bind | `frontend/src/pages/HelixOrgBotDetail.tsx` |
| Mutation hook (return new SessionID) | `frontend/src/services/helixOrgService.ts` |
| Bot restart handler | `api/pkg/org/interfaces/server/api/bots.go` (`restartBotAgent`) |
| New full-restart port | `api/pkg/org/interfaces/server/api/api.go` |
| In-proc client (compose delete + start) | `api/pkg/server/helix_org_inproc.go` (`StopExternalAgent`/`StartSession` are the pattern to mirror) |
| Delete-session op (existing) | `api/pkg/server/session_handlers.go` (`deleteSession`, line 246) |
| Start-session op (existing) | `api/pkg/server/session_handlers.go` (`StartExternalAgentSession`) |
| Runtime state pointer | `api/pkg/org/infrastructure/runtime/helix/state.go` (`SaveSession`) |
| Crash-recovery primitive (leave as-is) | `api/pkg/server/session_handlers.go` (`restartSessionContainer`) |

## Risks / Notes

- **Deleting the old session is the critical step** — without it, the exploratory
  singleton guard reuses the old ID and "restart" silently no-ops again. Test
  that a delete-then-start yields a genuinely new session ID.
- Use Helix's existing delete-session operation as-is (it already handles desktop
  teardown); do not add bespoke container/workspace teardown here.
- Validate the session is a `zed_external` agent before the full-restart path.
- The empty-session fallback must still provision + start for first-time use.
