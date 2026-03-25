# Requirements: Reduce Excessive Browser Notifications

## Problem

Users receive far more browser (OS-level) notification popups than expected. Two distinct bugs cause this:

1. **Double-pop per task**: When a spec task completes, two events are emitted (`specs_pushed` + `agent_interaction_completed`). The UI correctly collapses these into one card, but browser notifications fire for each raw event individually — so one task completion produces two OS popups.
2. **Re-fire on remount**: The deduplication sets (`shownRef` in `useBrowserNotifications`, `prevEventIdsRef` in `useAttentionEvents`) are in-memory React refs. They reset whenever `GlobalNotifications` remounts (e.g., on page navigation). All currently unacknowledged events then re-fire as new browser notifications.

## User Stories

- As a user, I should receive **one** browser notification when a task needs my attention, even if two related events arrive together.
- As a user, navigating between pages should not re-trigger browser notifications for events I've already seen.

## Acceptance Criteria

- [ ] Completing a spec task (spec push + agent interaction) produces exactly **1** browser notification, not 2.
- [ ] Navigating away and back does not re-fire browser notifications for existing unacknowledged events.
- [ ] New events that arrive after the user has been on the page still fire notifications promptly.
- [ ] Dismissing/acknowledging an event does not cause notifications to re-fire.
