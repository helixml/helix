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

- [x] Backend: live session → `ResetSession` runs then Activate enqueues a fresh session (`TestRestartBotAgent_ResetsThenActivatesExistingSession`).
- [x] Backend: no-live-session path activates without reset (`TestRestartBotAgent_ActivatesWithoutResetWhenNoSession`).
- [x] Backend: reset failure surfaces as 500 and does NOT activate (`TestRestartBotAgent_ResetFailureSurfaces`).
- [x] Backend: unknown bot → clean 404 before side effects (`TestRestartBotAgent_404OnUnknownBot`).
- [x] Backend: crash-recovery `restartSessionContainer` (in-chat) unchanged — existing `restart_session_container_test.go` still passes.

## Verification

- [x] Backend compiles cleanly: local `go build ./pkg/server/ ./pkg/org/...` and the inner stack's Air rebuild of the mounted source reached `running...` on the final code.
- [x] Backend unit tests pass (`go test ./pkg/org/interfaces/server/api/`), incl. reset→activate ordering, failure→500, 404, no-reset-first-start.
- [x] Crash-recovery primitive regression test passes (`TestRestartSessionContainerSuite`, CGO).
- [x] Frontend `tsc --noEmit` clean; vite transforms all modules.
- [x] Live e2e in inner Helix (HELIX_ORG_ENABLED=true, admin user + `helix-org` alpha feature, org+bot created via browser). **Destructive/reset half fully proven against real infra:**
  - Before restart: bot had live session `ses_…rnf0` (status `running`) + real desktop container `ubuntu-external-…rnf0`.
  - Clicked "Restart agent session" (shows updated caption + "RESTARTING…").
  - After: desktop container **gone** (StopExternalAgent tore it down); old session **soft-deleted** (`deleted_at` set — DeleteSession); runtime `session_id` pointer **cleared** (SaveSession ""); chat window shows **"No conversation yet for this bot."** (old transcript cleared). Screenshot: `screenshots/01-after-restart-chat-cleared.png`.
  - This directly resolves the original bug: the desktop no longer stays alive and the session/thread is removed.
- [ ] NOT observed e2e — the **new** session materialising in chat. Blocked by an environment issue: this inner Helix's desktop image fails to register the `claude-acp` agent ("Custom agent server `claude-acp` is not registered"), so the first activation's turn never completes; its `pollUntilDone` holds the per-worker dispatch queue (24h runaway guard), so the queued restart-activation can't run. In a healthy env (turns complete → queue idle) restart→Activate creates the fresh session immediately. Verify in staging.

## Follow-up finding (out of scope for this fix)

Routing the bot-page restart through `Activate` (the per-worker dispatch queue)
means that if a prior activation is still **in-flight** (e.g. a stuck/booting
desktop — the "system went down" case), the new activation queues behind it.
`pollUntilDone` treats a deleted-session `GetOutput` error as *transient* and
retries up to the 24h `ActivationRunawayGuard`, so a session deleted mid-flight
leaves the queue blocked. The reset still correctly tears the old desktop +
session down; only the *new* session creation is delayed. A tight fix would be
to make `pollUntilDone` treat "session not found" as terminal — but that touches
shared spawner code (all activations) and the not-found error isn't currently
typed, so it's flagged for a separate change rather than bundled here.
