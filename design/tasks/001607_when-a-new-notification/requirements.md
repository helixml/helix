# Requirements: Notification Bar Count Badge Turns Red on New Notifications

## User Story

As a user, when a new notification arrives, I want both the bell icon badge and the count pill in the notification panel header to turn red and stay red until I open the drawer — so I can clearly see there are unread notifications.

## Current Behavior

- The bell icon badge and the panel header count pill both use `hasNew` to drive their color (or should — see Issue 2).
- `hasNew` is derived from events that "appeared since last render" using a ref that's updated on every render. This means `hasNew` is only true for a single render cycle — the one where the new event first appears in the poll response. On the very next re-render it resets to false.
- Result: the badge *does* flash red for a fraction of a second (which is why it "sometimes works"), but it doesn't stay red.
- Additionally, the count pill in the panel header never reads `hasNew` at all — it always shows muted gray.

## Expected Behavior

- When a new (unacknowledged) notification arrives, both the bell badge and the panel header count pill turn red and remain red.
- Red state persists until the user opens the drawer, which acknowledges all events server-side.
- After opening the drawer and the next poll completes, both return to muted gray.

## Acceptance Criteria

- [ ] A new notification arrives → bell badge AND panel header count pill both turn red.
- [ ] Red state persists across re-renders (not just a brief flash).
- [ ] Opening the drawer triggers acknowledgement → both revert to gray on the next poll.
- [ ] No events → count pill is not shown (unchanged behavior).
