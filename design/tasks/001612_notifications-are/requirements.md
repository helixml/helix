# Requirements: Notifications Should Not Auto-Dismiss on Panel Open

## Problem

When the user opens the notification panel (clicks the bell icon), all unacknowledged notifications are automatically marked as acknowledged. This clears the red badge and changes their visual state, making it feel like the notifications were "dismissed" without any explicit user action.

The user expects notifications to persist until they explicitly dismiss them — either via the X button on an individual notification or the "Dismiss all" button.

## User Stories

**US-1:** As a user, when I open the notification panel, I want my notifications to remain in exactly the same state — so I can open the panel freely without accidentally "consuming" notifications.

**US-2:** As a user, I want notifications (and the red badge indicator) to only clear after I explicitly click the X button on a notification or click "Dismiss all."

**US-3:** As a user, I want clicking on a notification to navigate me to the relevant page without automatically dismissing the notification, so I can revisit it.

## Acceptance Criteria

- [ ] Opening the notification panel does NOT call `acknowledge()` for any event
- [ ] The red badge count does not change merely from opening the panel
- [ ] Individual notifications are only dismissed when the user clicks the X button
- [ ] All notifications are only dismissed when the user clicks "Dismiss all"
- [ ] Clicking a notification (navigating) does NOT auto-dismiss or auto-acknowledge it
- [ ] The `acknowledged_at` field is no longer set automatically on panel open
