# Implementation Tasks: Fix Restart Agent Session to Fully Reset Desktop and Context

## Backend — full-restart flow

- [ ] Add a dedicated full-restart port (e.g. `BotFullRestarter` / `FullRestart(ctx, orgID, botID)`) on the org api adapter (`api/pkg/org/interfaces/server/api/api.go`); do NOT reuse `restartSessionContainer`.
- [ ] Implement the full-restart in the in-proc helix client (`api/pkg/server/helix_org_inproc.go`):
  - [ ] Resolve the bot's current session (BotRuntime.State / exploratory lookup).
  - [ ] `StopDesktop(oldSessionID)` and surface failures (do not swallow).
  - [ ] Destroy the old session's workspace volume (via hydra executor) so no `threads.db`/agent state survives.
  - [ ] Retire the old exploratory session (delete the row, or unset `session_role`) so the singleton guard cannot reuse it and it is no longer resolved as current.
  - [ ] Create a new exploratory session via `StartExternalAgentSession` (`SessionRole="exploratory"`, `AgentType="zed_external"`, `AutoRestartOnCrash=true`) → new session ID + fresh desktop + fresh workspace.
  - [ ] Persist the new session ID via `SaveSession(orgID, botID, newSessionID)`.
- [ ] Update `restartBotAgent` (`api/pkg/org/interfaces/server/api/bots.go`) to call the full-restart flow and return the **new** session ID in `BotActivateDTO`.
- [ ] Keep the no-live-session path working (first-time start provisions project + starts fresh session).
- [ ] Verify workspace ZFS clone keying (per-session vs per-project) in `hydra_executor.go` and implement old-volume cleanup accordingly.
- [ ] Leave `restartSessionContainer` and the in-chat / spec-task callers unchanged.

## Frontend — switch chat window to the new session

- [ ] Ensure `useRestartBotAgent` (`frontend/src/services/helixOrgService.ts`) returns the `BotActivateDTO` so the new `SessionID` is available.
- [ ] In `handleRestartSession` (`frontend/src/pages/HelixOrgBotDetail.tsx`), after success set `chatSessionId` to the new `SessionID` (remount `EmbeddedSessionView` / re-run `fetchExistingWorkerSession` if needed) so the chat + desktop panels + WebSocket rebind to the new session and the old transcript disappears.
- [ ] Default the session panel to the Chat tab (`sessionTab='chat'`) after restart.
- [ ] Update success snackbar wording to "fresh session started"; keep error handling so a failed backend response surfaces an error snackbar. No confirmation dialog.

## Tests

- [ ] Backend: full restart deletes/retires the old exploratory session, mints a NEW session ID, and returns it.
- [ ] Backend: old desktop container + workspace volume are torn down; new session gets a fresh workspace.
- [ ] Backend: `StopDesktop` failure surfaces as an error (no false success).
- [ ] Backend: no-live-session path falls back to first-time start.
- [ ] Backend: crash-recovery `restartSessionContainer` (in-chat) still preserves session ID + `ZedThreadID`.

## Verification

- [ ] Manual: add a tool to a bot, click "Restart agent session", confirm a NEW session ID is created, the desktop is a fresh instance, and the chat window switches to a new empty session/thread with no prior context and the new tool available.
- [ ] Manual: confirm a tear-down failure shows an error (not "restart queued").
