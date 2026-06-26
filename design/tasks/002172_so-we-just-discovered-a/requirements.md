# Requirements: Friendly Error When a Project's Agent Is Missing

## Background

A project's default agent is a Helix **App** (`project.DefaultHelixAppID`). Sessions
resolve their agent from that App. When the App is **deleted**, the project keeps a
dangling reference and the session's stored `ZedAgentName` (e.g. `claude-acp`) points
at an agent that Zed can no longer register. Every interaction then fails with a raw,
unintelligible error bubbled up straight from the Rust fork:

```
Thread load failed: Failed to load thread: agent connect failed:
Other("Custom agent server `claude-acp` is not registered")
```

The user has no way to understand that the real problem is simply: **the agent this
project points to no longer exists.** Worse, because this error is currently classified
as transient, the queue auto-retries it forever — it can never succeed.

## User Stories

**US-1 — As a user whose project agent was deleted**, when I open or message a thread,
I want a plain-English message telling me the project has no agent configured and a clear
action to fix it, instead of a cryptic "agent connect failed / not registered" error.

**US-2 — As a user**, I don't want the system to silently retry a permanently-broken
agent connection. The failure should be terminal with a "Configure agent" call to action,
not an endless "retrying now…" spinner.

**US-3 — As a user deleting an agent**, I want the system to keep project state
consistent — a project should not be left pointing at an agent that no longer exists.

## Acceptance Criteria

1. **Friendly message.** When a thread load / agent connect fails because the agent is
   not registered / not configured, the error shown to the user reads (wording final in
   design), e.g.:
   *"This project doesn't have an agent configured. Go to the project's Agent settings to
   choose or create one."*
   The raw `claude-acp is not registered` string is no longer the user-facing text.

2. **Actionable CTA.** The failed interaction in the prompt input renders a button/link
   that takes the user to where they configure the project's agent (Project Settings →
   Agent, or the Agents section), visually distinct from the existing "Restart" crash CTA.

3. **No futile retries.** The "agent not configured" error is treated as terminal
   (auto-retry suppressed), not as a transient error that loops.

4. **Consistent state on delete.** Deleting an agent App clears the
   `DefaultHelixAppID` reference on any project that pointed to it, so projects are never
   left with a dangling agent pointer. (Existing already-broken projects are also handled
   gracefully by criteria 1–3.)

5. **Regression safety.** Genuine transient failures (booting agent, reconnecting socket)
   and genuine hard crashes (`Claude Agent process exited`, `Session not found`) keep
   their existing behavior — only the "not registered / not configured" class changes.

## Out of Scope

- Investigating *how* the agent App was deleted in the first place (separate concern).
- Auto-recreating or auto-assigning a replacement agent.
- Changes to the Zed/Rust fork beyond what is needed to classify the error (the friendly
  translation can live entirely in the Helix Go + frontend layers).
