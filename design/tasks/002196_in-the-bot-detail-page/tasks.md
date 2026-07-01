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
- [ ] NOT DONE — live end-to-end in the inner Helix: blocked because `HELIX_ORG_ENABLED=false` in this instance (no bots/activations tables, bot detail page absent). Flipping the operator feature flag + restarting the shared stack was deemed out of scope for a verification step. The reset→activate ordering and failure/first-start paths are covered by unit tests; the integration path (StopExternalAgent → DeleteSession → clear pointer → Activate producing a fresh live desktop) has NOT been exercised against a live Zed session — flag for reviewer/staging verification.
