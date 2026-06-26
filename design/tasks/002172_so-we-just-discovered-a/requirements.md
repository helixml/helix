# Requirements: Handle Deleted Agents Gracefully in Projects and Sessions

## Background

An agent in Helix is an **App**. It can be referenced in two places:

- **Project default agent** — `project.DefaultHelixAppID`. Controls which agent **new**
  sessions get.
- **Session agent** — an already-started session is bound to its own agent via
  `session.ParentApp` (and, for spec tasks, the spec task's `HelixAppID`). Changing the
  project default does **not** re-point existing sessions.

When the agent App is **deleted**, both references dangle. Two distinct user-visible
failures result:

**Facet A — the project default is gone.** New sessions / thread loads fail with a raw,
unintelligible error bubbled straight from the Rust fork:

```
Thread load failed: Failed to load thread: agent connect failed:
Other("Custom agent server `claude-acp` is not registered")
```

The failure is currently classified as transient, so the queue **auto-retries forever**
and the user only sees "retrying now…" with no idea what's wrong.

**Facet B — an already-started session's agent is ripped out.** Even after the project is
given a new default agent, every existing session that pointed at the deleted agent is
stranded — the user can only recover by re-pointing each session individually. Today they
discover this only by typing a message and watching it fail.

## User Stories

**US-1 — Project default missing (Facet A).** As a user whose project's default agent was
deleted, when I start/continue a thread I want a plain-English message saying the project
has no agent configured, with an action to fix it — not a cryptic "not registered" error.

**US-2 — No futile retries (Facet A).** As a user, I don't want the system to retry a
permanently-broken agent connection forever; the failure should be terminal with a clear
call to action.

**US-3 — Stranded session (Facet B).** As a user opening a session whose assigned agent no
longer exists, I want to be told **proactively, before I type anything**, that the session
has no agent and that I must assign one before continuing.

**US-4 — Easy reassignment (Facet B).** As that user, I want to assign an existing agent
to the session right there, and then continue the conversation seamlessly.

**US-5 — Consistent state on delete.** As a user deleting an agent, I don't want to leave
projects pointing at an agent that no longer exists.

## Acceptance Criteria

1. **Friendly message (A).** When a thread load / agent connect fails because the agent is
   not registered / not configured, the user-facing error is plain English (wording in
   design), e.g. *"This project doesn't have an agent configured. Open the project's Agent
   settings to choose or create one."* The raw `claude-acp is not registered` string is no
   longer the user-facing text.

2. **Actionable CTA (A).** The failed interaction renders a button/link to where the
   project agent is configured, visually distinct from the existing "Restart" crash CTA.

3. **No futile retries (A).** The "agent not configured" class is terminal (auto-retry
   suppressed), not transient.

4. **Proactive session block (B).** When a session's assigned agent no longer exists, the
   session view shows a banner **before the user sends anything**:
   *"There is currently no agent assigned to this session. Before we can proceed, please
   assign one."* The message input / send is **disabled** while in this state, so a doomed
   message can't be sent.

5. **In-place reassignment (B).** From that banner the user can pick an existing agent
   (reusing the existing switch-agent flow). After assignment the banner clears, input
   re-enables, and the conversation continues on the same session.

6. **Consistent state on delete (E).** Deleting an agent App clears `DefaultHelixAppID` on
   any project that referenced it. (Already-stranded projects/sessions are still handled by
   criteria 1–5.)

7. **Regression safety.** Genuine transient failures (booting/reconnecting agent) and
   genuine hard crashes (`Claude Agent process exited`, `Session not found`) keep their
   current behavior — only the "agent missing / not configured" class is new.

## Out of Scope

- Investigating *how* the agent App was deleted in the first place.
- Auto-recreating or auto-assigning a replacement agent (the user chooses).
- Rust/Zed-fork changes beyond keeping the existing recognizable error string.
- Bulk "reassign all sessions of deleted agent X" tooling (per-session assign is enough
  for v1; the proactive block makes the manual path safe and obvious).
