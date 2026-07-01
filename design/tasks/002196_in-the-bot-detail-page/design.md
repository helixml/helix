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
- **Desktop + workspace.** `StartDesktop` (`hydra_executor.go`) ZFS-clones the
  workspace and mounts it (incl. `threads.db`). Crash-recovery `StopDesktop`
  **preserves** that ZFS volume so it can be remounted — which is exactly what we
  must NOT do for a full restart.

## Proposed change

Give `restartBotAgent` a dedicated **full-restart** backend flow (a new port on
the org api adapter, e.g. `SessionRestarter.FullRestart(ctx, orgID, botID)` or a
new `BotFullRestarter`), implemented in the in-proc helix client. Do **not**
reuse `restartSessionContainer`. Algorithm:

1. **Resolve the bot's current session** from `BotRuntime.State` /
   exploratory lookup. If none exists, skip to step 4 (first-time start).
2. **Fully tear down the old session:**
   - `StopDesktop(ctx, oldSessionID)` — stop/delete the container. For this path,
     tear-down failure should be surfaced (not silently swallowed).
   - **Destroy the old session's workspace volume** so no `threads.db` / agent
     state survives (see "Workspace teardown" below).
   - **Retire the old exploratory session** so it is not resolved as current and
     cannot be reused by the singleton guard — delete the session row (or flip
     its `session_role` off "exploratory"). This is the load-bearing step that
     makes the next start mint a *new* ID instead of reusing the old one.
3. **Create a brand-new session** on the same project via the same primitive the
   spawner/cron use — `StartExternalAgentSession` with
   `SessionRole="exploratory"`, `AgentType="zed_external"`,
   `AutoRestartOnCrash=true`. Because the old row is gone, this mints a **new
   session ID**, `StartDesktop` provisions a fresh container, and a fresh ZFS
   workspace clone is created.
4. **Persist the new session ID** into runtime state via `SaveSession(ctx,
   store, orgID, botID, newSessionID)` so the mirror and future activations
   resolve the new session.
5. **Return the new session ID** in `BotActivateDTO.SessionID`.

First-time start (no existing session): steps 2 is a no-op; step 3 provisions
the project (via `Activate`/ensurer) and starts the first session.

### Workspace teardown

Crash-recovery `StopDesktop` intentionally preserves the ZFS workspace volume.
For a full restart we need the old workspace gone. Two candidate mechanisms:

- **A (recommended):** because the new session has a new session ID, its
  `StartDesktop` creates a *new* per-session ZFS clone — so the new desktop is
  already pristine regardless of the old volume. Add an explicit destroy of the
  **old** session's volume (via the hydra executor / `DeleteDevContainer` +
  volume delete) so it doesn't leak. Confirm during implementation whether the
  ZFS clone is keyed per-session (then this is automatic + cleanup) or per-
  project (then explicit reset is required).
- **B:** delete + re-clone the project workspace volume in place. Heavier and
  only needed if the workspace clone is project-keyed. Prefer A.

**Open item for implementation:** verify in `hydra_executor.go` whether the
workspace ZFS clone is keyed by session ID or project ID — this decides whether
old-volume cleanup is just hygiene (A) or mandatory for freshness (B).

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
| In-proc client (full-restart impl) | `api/pkg/server/helix_org_inproc.go` |
| New session primitive | `api/pkg/server/session_handlers.go` (`StartExternalAgentSession`) |
| Exploratory lookup / retire | `api/pkg/store/store_sessions.go` (`GetProjectExploratorySession`) |
| Runtime state pointer | `api/pkg/org/infrastructure/runtime/helix/state.go` (`SaveSession`) |
| Desktop + workspace lifecycle | `api/pkg/external-agent/hydra_executor.go` (`StopDesktop`/`StartDesktop`) |
| Crash-recovery primitive (leave as-is) | `api/pkg/server/session_handlers.go` (`restartSessionContainer`) |
| Full teardown reference | `api/pkg/org/application/lifecycle/lifecycle.go` (`Delete`) |

## Risks / Notes

- **Session retirement is the critical step** — without deleting/retiring the old
  exploratory row, the singleton guard reuses the old ID and "restart" silently
  no-ops again. Test this explicitly.
- Deleting the old session row must cascade/clean its interactions and container
  metadata; reuse the project-delete teardown helpers where possible.
- Confirm workspace ZFS clone keying (per-session vs per-project) before choosing
  cleanup A vs B.
- Validate the session is a `zed_external` agent before the full-restart path.
- The empty-session fallback must still provision + start for first-time use.
