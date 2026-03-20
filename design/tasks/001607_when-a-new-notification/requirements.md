# Requirements: Notification Bar Count Badge Turns Red on New Notifications

## User Story

As a user, when a new notification arrives, I want the count badge inside the notification panel header to turn red — matching the bell icon badge — so I can immediately see there are unread notifications even when the panel is open.

## Current Behavior

- The bell icon badge turns red (`color="error"`) when `hasNew` is true (new unacknowledged events).
- The count badge in the panel header ("Needs Attention" + count pill) stays gray (`rgba(255,255,255,0.06)` background, `rgba(255,255,255,0.5)` text) regardless of whether there are new events.

## Expected Behavior

- When `hasNew` is true, the count pill in the panel header should also turn red (use `error`/red background color).
- When all events are acknowledged (`hasNew` is false), the count pill returns to its muted gray style.
- This mirrors the same `hasNew ? 'error' : 'default'` logic already used on the bell badge.

## Acceptance Criteria

- [ ] A new notification arrives → panel header count pill changes to red background.
- [ ] Opening the drawer (which acknowledges all events) → count pill reverts to muted gray.
- [ ] No events → count pill is not shown (unchanged behavior).
- [ ] The bell icon badge behavior is unaffected.
