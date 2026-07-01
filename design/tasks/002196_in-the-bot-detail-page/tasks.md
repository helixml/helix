# Implementation Tasks: Fix Restart Agent Session to Fully Reset Desktop and Context

## Backend — full-restart flow (compose existing operations)

- [ ] Add a dedicated full-restart port (e.g. `BotFullRestarter` / `FullRestart(ctx, orgID, botID)`) on the org api adapter (`api/pkg/org/interfaces/server/api/api.go`); do NOT reuse `restartSessionContainer` and do NOT touch container/workspace internals.
- [ ] Add a thin in-proc `DeleteSession` wrapper (`api/pkg/server/helix_org_inproc.go`) that calls the existing `deleteSession` handler — mirror the existing `StopExternalAgent` wrapper.
- [ ] Implement the full-restart in the in-proc helix client by composing existing ops:
  - [ ] Resolve the bot's current session (BotRuntime.State / exploratory lookup).
  - [ ] Delete the old session via the existing delete-session op (tears down its desktop, removes the exploratory singleton). Surface failures (do not swallow).
  - [ ] Create a new session via the existing `StartSession` primitive → new session ID + fresh desktop + fresh MCP services.
  - [ ] Persist the new session ID via `SaveSession(orgID, botID, newSessionID)`.
- [ ] Update `restartBotAgent` (`api/pkg/org/interfaces/server/api/bots.go`) to call the full-restart flow and return the **new** session ID in `BotActivateDTO`.
- [ ] Keep the no-live-session path working (first-time start provisions project + starts fresh session).
- [ ] Leave `restartSessionContainer` and the in-chat / spec-task callers unchanged.

## Frontend — switch chat window to the new session

- [ ] Ensure `useRestartBotAgent` (`frontend/src/services/helixOrgService.ts`) returns the `BotActivateDTO` so the new `SessionID` is available.
- [ ] In `handleRestartSession` (`frontend/src/pages/HelixOrgBotDetail.tsx`), after success set `chatSessionId` to the new `SessionID` (remount `EmbeddedSessionView` / re-run `fetchExistingWorkerSession` if needed) so the chat + desktop panels + WebSocket rebind to the new session and the old transcript disappears.
- [ ] Default the session panel to the Chat tab (`sessionTab='chat'`) after restart.
- [ ] Update success snackbar wording to "fresh session started"; keep error handling so a failed backend response surfaces an error snackbar. No confirmation dialog.

## Tests

- [ ] Backend: full restart deletes the old session, mints a NEW session ID, and returns it.
- [ ] Backend: the new session is a fresh desktop (old session no longer resolvable / reused).
- [ ] Backend: delete-session failure surfaces as an error (no false success).
- [ ] Backend: no-live-session path falls back to first-time start.
- [ ] Backend: crash-recovery `restartSessionContainer` (in-chat) still preserves session ID + `ZedThreadID`.

## Verification

- [ ] Manual: add a tool to a bot, click "Restart agent session", confirm a NEW session ID is created, the desktop is a fresh instance, and the chat window switches to a new empty session/thread with no prior context and the new tool available.
- [ ] Manual: confirm a tear-down failure shows an error (not "restart queued").
