# Implementation Tasks: Fix Restart Agent Session to Fully Reset Desktop and Context

## Backend — full-restart flow (compose existing operations)

Key discoveries during implementation:
- `store.DeleteSession` only deletes the DB row — it does **not** stop the
  desktop. Must call `StopExternalAgent` (→ `StopDesktop`) **before** deleting,
  else the container leaks (the original bug).
- `StartExternalAgentSession` **reuses** an existing exploratory session, so the
  old exploratory row **must be deleted** for a new session ID to be minted.
- The spawner's `ensureSession` reads the persisted `SessionID` pointer directly
  and calls `ClearSession` on it; if we delete the session but leave the pointer,
  that `ClearSession` errors. So we **must clear the pointer** (`SaveSession ""`).
- Reuse the existing `Activate` path for creation — the spawner already resolves
  provider/model/prompt; no need to replicate that logic or call `StartSession`.

- [~] Add `BotSessionResetter` port (`ResetSession(ctx, orgID, botID, sessionID)`) on the org api adapter (`api/pkg/org/interfaces/server/api/api.go`); remove the now-dead `SessionRestarter` port (crash-recovery restart is not what the bot page wants).
- [~] Add a thin in-proc `DeleteSession` wrapper (`api/pkg/server/helix_org_inproc.go`) that calls the existing `deleteSession` handler — mirror the existing `StopExternalAgent` wrapper.
- [~] Implement `ResetSession` in the in-proc helix client by composing existing ops: `StopExternalAgent(sessionID)` → `DeleteSession(sessionID)` → `SaveSession(orgID, botID, "")`. Surface failures.
- [~] Update `restartBotAgent` (`api/pkg/org/interfaces/server/api/bots.go`): resolve session → if live session, `ResetSession` → then always `Activate` (provisions a brand-new session + fresh desktop; also the first-time path).
- [ ] Remove the now-dead `inProcHelixClient.RestartSession` and re-wire the composition root (`helix_org.go`) to `BotSessionResetter`.
- [ ] Leave `restartSessionContainer` / `restartCrashedAgentThread` (in-chat / spec-task) unchanged.

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
