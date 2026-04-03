# Requirements: Immediately Show Desktop "Starting" State on Start Planning Click

## Problem

When the user clicks "Start Planning", they see the button's own "Starting..." spinner, but the desktop viewer's "Starting Desktop..." spinner (with `CircularProgress`) does not appear until after a poll interval completes. The desktop viewer is only rendered once `planning_session_id` is populated on the task — which requires waiting for the backend to create the session AND for the frontend polling to catch the update (up to ~10s default, or ~3s with the aggressive post-click polling).

## User Stories

**As a user clicking "Start Planning",**
I want to immediately see the "Starting Desktop..." spinner in the task card,
so I have instant visual feedback that the desktop is being provisioned.

## Acceptance Criteria

1. Within ~200ms of clicking "Start Planning" (i.e., as soon as the API call succeeds), the desktop viewer area shows the "Starting Desktop..." `CircularProgress` spinner — not just the button's own spinner.
2. The desktop viewer's starting state is shown optimistically, before the background poll confirms `planning_session_id`.
3. If the API call fails, the optimistic starting state is rolled back and no desktop viewer is shown.
4. Once the real `planning_session_id` arrives (via polling), the component transitions normally to real desktop status polling.
5. No regression in error handling or timeout detection (120s "Desktop may have failed to start" message still works).
