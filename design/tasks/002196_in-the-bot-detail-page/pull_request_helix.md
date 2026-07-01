# Fix "Restart agent session" to start a genuinely fresh session and desktop

## Summary

The bot detail page's Advanced → **Restart agent session** button was wired to
the crash-recovery restart primitive (`restartSessionContainer`), which
*intentionally preserves* the session: it kept the same session ID, reused the
old Zed thread (`ZedThreadID`) and workspace, and only reset crashed prompts. So
operators saw the desktop stay alive and the thread never clear — the opposite of
the button's intent (get a fresh desktop + context so newly added tools/MCP
services take effect, or to recover a stuck instance).

This change makes the button perform a **full restart** by composing existing
high-level Helix operations — no container/workspace internals:

1. Stop the desktop (`StopExternalAgent`, best-effort — never resumed).
2. Delete the old session row (`deleteSession`). This is load-bearing: an
   exploratory session is a project singleton that `StartExternalAgentSession`
   would otherwise reuse, so deletion is what forces a brand-new session ID.
3. Clear the persisted session pointer (`SaveSession ""`) so the spawner starts
   fresh instead of trying to clear the now-deleted session.
4. `Activate` (existing) — the spawner mints a new session, starts a fresh
   desktop, and re-reads the bot's current tools/MCP surface.

The frontend then switches the chat window to the new session: it drops the old
transcript, shows the Chat tab, and polls for the freshly-created exploratory
session (created asynchronously) before binding to it. No confirmation dialog.

The in-chat / spec-task crash-recovery restart (`restartCrashedAgentThread` →
`restartSessionContainer`) is unchanged.

## Changes

- `api/pkg/org/interfaces/server/api/api.go`: replace the `SessionRestarter` port
  with `BotSessionResetter` (`ResetSession(ctx, orgID, botID, sessionID)`).
- `api/pkg/org/interfaces/server/api/bots.go`: `restartBotAgent` now resets the
  live session (if any) then always activates a fresh one.
- `api/pkg/server/helix_org_inproc.go`: add `DeleteSession` wrapper; remove dead
  `RestartSession`.
- `api/pkg/server/helix_org.go`: add `botSessionResetter` adapter
  (StopExternalAgent → DeleteSession → clear pointer) and wire it.
- `frontend/src/pages/HelixOrgBotDetail.tsx`: rewrite `handleRestartSession` to
  show the fresh session; update the Advanced caption.
- `frontend/src/services/helixOrgService.ts`: update `useRestartBotAgent` docs.
- Tests: rewrite `restart_bot_test.go` for reset→activate, failure→500, 404,
  first-start.

## Testing

- `go test ./pkg/org/interfaces/server/api/` — green (new handler behaviour).
- `TestRestartSessionContainerSuite` — green (crash-recovery unchanged).
- `go build ./pkg/server/ ./pkg/org/...` and frontend `tsc --noEmit` — clean.
- **Live e2e** in inner Helix (org enabled, bot created via browser):
  - Reset half: the running desktop container was torn down, the old session
    soft-deleted, the runtime pointer cleared, and the chat window cleared to
    "No conversation yet".
  - Create half: a new session (new id) + new desktop container were minted, the
    pointer updated, and the chat window rebound to the new session's fresh
    transcript (manual-wake trigger), confirming a genuinely new session/thread.

## Screenshots

![After restart — chat cleared](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002196_in-the-bot-detail-page/screenshots/01-after-restart-chat-cleared.png)
![Fresh session shown](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002196_in-the-bot-detail-page/screenshots/02-fresh-session-shown.png)

## Known follow-up (not in this PR)

Restart now routes through the per-worker dispatch queue (via `Activate`). If a
prior activation is still *in-flight* when you restart (e.g. a stuck/booting
desktop), the new activation queues behind it; `pollUntilDone` treats a
deleted-session poll error as transient (retries to the 24h runaway guard), so
the new session is delayed. The reset still tears the old desktop + session down
correctly. A tight fix (treat "session not found" as terminal in `pollUntilDone`)
touches shared spawner code and is left for a separate change.
