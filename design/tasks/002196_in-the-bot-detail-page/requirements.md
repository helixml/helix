# Requirements: Fix Restart Agent Session to Fully Reset Desktop and Context

## Background

The org **Bot Detail page** (`frontend/src/pages/HelixOrgBotDetail.tsx`) has an
**Advanced** section with a **"Restart agent session"** button. Its intended
purpose: completely remove the bot's previous agent session across all threads
and bring up a **fresh desktop and context**. It is used when new tools are
added to a bot, or when the running instance gets stuck.

**The bug:** clicking the button does *not* start a fresh session. The
underlying desktop container stays alive and the conversation thread is never
cleared. The button reports success ("restart queued") regardless.

### Why it happens (root cause)

The button calls `POST /api/v1/orgs/{org}/bots/{id}/restart-agent`
(`restartBotAgent`), which delegates to the shared **crash-recovery** primitive
`restartSessionContainer` (`api/pkg/server/session_handlers.go`). That primitive
is designed to *recover a crashed agent while keeping its context*, so it
**deliberately**:

- Preserves `session.Metadata.ZedThreadID` → Zed reloads the *same* thread.
- Preserves the workspace ZFS volume (`threads.db` + agent session on disk).
- Leaves DB interactions untouched. Only crashed prompts are reset.

That is the exact opposite of "fresh desktop and context." It never calls the
`ClearSession` primitive (`POST /sessions/{id}/clear`), which is the code path
that actually wipes interactions and resets `ZedThreadID = ""`.

Two additional gaps make it look like "nothing happened":

- `StopDesktop` errors are swallowed (logged at warn, flow continues), so a
  container that fails to tear down still yields a success response.
- If the bot's session can't be resolved (`BotRuntime.State` empty) the handler
  falls back to `Activate`, which *continues* the existing session and cannot
  recover a stuck container.

## User Stories

**US-1 — Fresh start on demand.** As an org admin on the Bot Detail page, when I
click "Restart agent session" I want the bot's current session fully reset — all
threads/conversation cleared and a brand-new desktop started — so that newly
added tools take effect and stuck instances are genuinely recovered.

**US-2 — Honest feedback.** As an admin, if the desktop fails to tear down or a
step fails, I want the UI to tell me it failed rather than falsely report
success.

**US-3 — Guard against accidents.** As an admin, since this is destructive
(context is discarded), I want a confirmation prompt before it runs.

## Acceptance Criteria

1. Clicking "Restart agent session" on a bot with a live session:
   - Clears all session interactions and resets the Zed thread
     (`ZedThreadID = ""`) so the next message opens a **new** thread.
   - Tears down the running desktop container and starts a **fresh** one.
   - Does **not** preserve the previous conversation/thread.
2. After restart, sending a message to the bot lands on a new, empty thread on a
   newly started desktop (no prior conversation carried over).
3. If tear-down or any step fails, the API returns an error and the UI shows an
   error snackbar (no false "restart queued" success).
4. The button shows a confirmation dialog warning that current context will be
   permanently discarded before proceeding.
5. First-time start (bot has no live session) still works — falls back to a
   normal activation.
6. The crash-recovery `restartSessionContainer` behavior used by other surfaces
   (in-chat restart button, spec-task page) is unchanged, OR the divergence is
   made explicit via a "fresh vs preserve" parameter (see design).

## Out of Scope

- Changing the in-chat `/sessions/{id}/restart-agent` crash-recovery semantics.
- Auto-restart / crash-loop handling.
- The `PreserveContext` per-bot policy used by the spawner on re-activation.
