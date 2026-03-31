# Requirements: Auto-Start Desktop When Commenting on a Spec

## Background

When a user submits a comment on a spec design review, the comment is queued in the database for the AI agent to process. The agent runs inside a "desktop" session (a containerized remote desktop). If the desktop is stopped, the agent cannot respond — and after a 2-minute timeout the comment is marked "Agent did not respond".

Users expect that submitting a comment will trigger the desktop to start automatically, rather than silently failing.

## User Stories

**US-1: Comment while desktop is stopped (session exists)**
> As a reviewer with an existing but stopped planning session, when I submit a comment on a spec, I want the desktop to resume automatically, so my comment gets a response without me manually clicking "Start Desktop".

**US-2: Comment while desktop is already running**
> As a reviewer whose desktop is already running, when I submit a comment, I want no change in behavior — the comment should be submitted as normal.

## Acceptance Criteria

- **AC-1:** When the user submits a comment and the desktop is stopped (`sandboxState === "absent"`), the system automatically calls the resume API (`POST /api/v1/sessions/{sessionId}/resume`) before or alongside comment submission.
- **AC-2:** The comment is submitted to the database queue regardless of desktop state (the queue persists; the agent processes it once connected).
- **AC-3:** The UI shows feedback to the user that the desktop is being started (e.g. a transient message or status indicator).
- **AC-4:** If starting the desktop fails, the comment is still submitted and the user is shown an error message about the desktop start failure, but the comment is not lost.
- **AC-5:** If the desktop is already running, behavior is unchanged.
