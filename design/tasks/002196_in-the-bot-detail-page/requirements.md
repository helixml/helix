# Requirements: Fix Restart Agent Session to Fully Reset Desktop and Context

## Background

The org **Bot Detail page** (`frontend/src/pages/HelixOrgBotDetail.tsx`) has an
**Advanced** section with a **"Restart agent session"** button. Its intended
purpose: completely remove the bot's previous agent session and bring up a
**brand-new session, desktop and workspace** — everything fresh. It is used when
new tools are added to a bot, or when the running instance gets stuck.

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
- Preserves the workspace volume (`threads.db` + agent session on disk).
- Reuses the **same session ID**. Only crashed prompts are reset.

That is the opposite of the intended "completely new session and desktop."

Two additional gaps make it look like "nothing happened":

- `StopDesktop` errors are swallowed (logged at warn, flow continues), so a
  container that fails to tear down still yields a success response.
- If the bot's session can't be resolved (`BotRuntime.State` empty) the handler
  falls back to `Activate`, which resolves the **existing** exploratory session
  rather than minting a new one.

### Key constraint discovered

Exploratory sessions are a **project-scoped singleton**: at most one row per
`(project_id, session_role="exploratory")`, and `StartExternalAgentSession`
**reuses** an existing one rather than minting a parallel row (a deliberate fix).
The mirror resolves "the current session" via `GetProjectExploratorySession`
(`ORDER BY created DESC`). So to get a genuinely **new session ID**, the old
exploratory session must first be retired/deleted; only then does a new
`StartExternalAgentSession` mint a fresh row + fresh desktop + fresh workspace.

## User Stories

**US-1 — Full restart on demand.** As an org admin on the Bot Detail page, when I
click "Restart agent session" I want the bot's current session, desktop and
workspace **completely destroyed and replaced with brand-new ones** (new session
ID, new container, fresh context) — so newly added tools take effect and a stuck
instance is genuinely gone, not resumed.

**US-2 — Honest feedback.** As an admin, if a step fails, I want the UI to tell
me it failed rather than falsely report success.

**US-3 — See the fresh session.** As an admin, after I click "Restart agent
session", I want the chat window on the Bot Detail page to immediately switch to
the **new** session/thread (and its fresh desktop) — not the old transcript — so
I can see the restart worked and start chatting on the clean session. No
confirmation prompt is needed; clicking the button just does it.

## Acceptance Criteria

1. Clicking "Restart agent session" on a bot with a live session:
   - Tears down the running desktop container **and** its workspace volume for
     the old session (nothing from the old session is reused).
   - Retires/deletes the old exploratory session so it is no longer resolved as
     "current".
   - Creates a **new** exploratory session (new session ID) on the same
     project, with a fresh desktop container and fresh workspace.
   - Persists the new session ID into the bot's runtime state so the mirror and
     future activations resolve the new session.
2. The response returns the **new** session ID.
3. After restart, sending a message to the bot lands on the new, empty
   session/thread on the newly started desktop — no prior conversation, and
   newly added tools are present.
4. After a successful restart, the Bot Detail chat window switches to the new
   session ID and shows the empty transcript + fresh desktop stream. Clicking the
   button runs immediately with **no** confirmation dialog.
5. If any step fails, the API returns an error and the UI shows an error
   snackbar (no false "restart queued" success).
6. First-time start (bot has no live session yet) still works — provisions the
   project and starts a fresh session.
7. The crash-recovery `restartSessionContainer` behavior used by other surfaces
   (in-chat restart button, spec-task page) is **unchanged** — the full-restart
   flow is specific to the Bot Detail page button.

## Out of Scope

- Changing the in-chat `/sessions/{id}/restart-agent` crash-recovery semantics.
- Auto-restart / crash-loop handling.
- The `PreserveContext` per-bot policy used by the spawner on re-activation.
