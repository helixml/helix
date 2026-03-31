# Requirements: Auto-Start Desktop When Sending a Message

## Background

When a user sends any message to a session — whether a spec design review comment or a plain chat message in the session view — the message is queued for the AI agent to process. The agent runs inside a "desktop" session (a containerized remote desktop). If the desktop is stopped, the agent cannot respond — and after a 2-minute timeout the message is marked "Agent did not respond".

Users expect that sending any message will trigger the desktop to start automatically, rather than silently failing.

## User Stories

**US-1: Send message while desktop is stopped (session exists)**
> As a user with an existing but stopped session, when I send any message (spec comment or chat message), I want the desktop to resume automatically, so my message gets a response without me manually clicking "Start Desktop".

**US-2: Send message while desktop is already running**
> As a user whose desktop is already running, when I send a message, I want no change in behavior — the message should be submitted as normal.

## Acceptance Criteria

- **AC-1:** When the user sends any message (spec comment or session chat) and the desktop is stopped (`sandboxState === "absent"`), the system automatically calls the resume API (`POST /api/v1/sessions/{sessionId}/resume`) before or alongside message submission.
- **AC-2:** The message is submitted regardless of desktop state (the queue persists; the agent processes it once connected).
- **AC-3:** The UI shows feedback to the user that the desktop is being started (e.g. a transient message or status indicator).
- **AC-4:** If starting the desktop fails, the message is still submitted and the user is shown an error message about the desktop start failure, but the message is not lost.
- **AC-5:** If the desktop is already running, behavior is unchanged.
