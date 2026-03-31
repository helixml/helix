# Requirements

## Problem

When "Start Planning" is clicked, the desktop viewer briefly flashes a "stopped" state (showing a start button) for 1–2 seconds before transitioning to "Starting Desktop". This is confusing and looks like an error.

## Root Cause

The sequence of events:
1. User clicks "Start Planning" → task status becomes `QueuedSpecGeneration`, but `planning_session_id` is still empty.
2. `useSandboxState("")` is called with an empty session ID → returns "absent/stopped" state → UI shows the stopped desktop with a start button.
3. Backend creates the session asynchronously, populates `planning_session_id`.
4. Frontend polls (every 3s) → gets real session → detects "starting" state → UI updates.

## User Stories

**As a user**, when I click "Start Planning", I want the desktop to immediately show "Starting Desktop" so I know the action was received and is in progress.

**As a user**, I should not see the desktop's stopped state (with a clickable start button) after I've already initiated planning, as it looks broken.

## Acceptance Criteria

- [ ] Immediately after clicking "Start Planning", the desktop viewer shows "Starting Desktop" (not the stopped state).
- [ ] No flash of the stopped/absent desktop state occurs between click and the actual starting state.
- [ ] The transition feels instantaneous — no 1–2 second delay before the correct UI appears.
- [ ] If planning genuinely cannot start (error), the stopped state is appropriate to show.
