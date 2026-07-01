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

- [x] Add `BotSessionResetter` port (`ResetSession(ctx, orgID, botID, sessionID)`) on the org api adapter (`api/pkg/org/interfaces/server/api/api.go`); removed the now-dead `SessionRestarter` port.
- [x] Add a thin in-proc `DeleteSession` wrapper (`api/pkg/server/helix_org_inproc.go`) mirroring `StopExternalAgent`.
- [x] Implement `ResetSession` as a `botSessionResetter` adapter at the composition root (`helix_org.go`) — it has both the in-proc client and the org store: `StopExternalAgent` (best-effort) → `DeleteSession` (fatal) → `SaveSession(orgID, botID, "")` (fatal). (Store access stays out of the api package; the in-proc client alone doesn't hold the org runtime-state store.)
- [x] Update `restartBotAgent`: resolve session → if live session, `ResetSession` → then always `Activate`.
- [x] Removed the now-dead `inProcHelixClient.RestartSession`; re-wired `helix_org.go` to `BotSessionResetter`.
- [x] Left `restartSessionContainer` / `restartCrashedAgentThread` (in-chat / spec-task) unchanged.
- [x] Rewrote `restart_bot_test.go` for the new reset→activate behavior; `go test ./pkg/org/interfaces/server/api/` green.

## Frontend — switch chat window to the new session

Note: the new session is created **asynchronously** by the spawner after the
restart POST returns, so the response can't carry the new id. Instead the
handler drops the stale transcript and **polls** `fetchExistingWorkerSession`
until a different (fresh) exploratory session is resolvable, then binds to it.

- [x] `useRestartBotAgent` (`helixOrgService.ts`) already returns the `BotActivateDTO`; updated its comment for the new behavior.
- [x] Rewrote `handleRestartSession` (`HelixOrgBotDetail.tsx`): clear `chatSessionId` (drops old transcript), switch to Chat tab, then poll `fetchExistingWorkerSession(projectID)` until a new session id appears and set it (remounts `EmbeddedSessionView` + rebinds WebSocket).
- [x] Default the session panel to the Chat tab (`sessionTab='chat'`) after restart.
- [x] Success snackbar now says "Fresh agent session started…"; error handling unchanged; no confirmation dialog. Updated the Advanced caption too.
- [x] `tsc --noEmit` passes (exit 0); vite transforms all modules. `yarn build` can't write the reserved `frontend/dist` bind-mount in this env (frontend runs via Vite HMR) — not a code issue.

## Tests

- [ ] Backend: full restart deletes the old session, mints a NEW session ID, and returns it.
- [ ] Backend: the new session is a fresh desktop (old session no longer resolvable / reused).
- [ ] Backend: delete-session failure surfaces as an error (no false success).
- [ ] Backend: no-live-session path falls back to first-time start.
- [ ] Backend: crash-recovery `restartSessionContainer` (in-chat) still preserves session ID + `ZedThreadID`.

## Verification

- [ ] Manual: add a tool to a bot, click "Restart agent session", confirm a NEW session ID is created, the desktop is a fresh instance, and the chat window switches to a new empty session/thread with no prior context and the new tool available.
- [ ] Manual: confirm a tear-down failure shows an error (not "restart queued").
