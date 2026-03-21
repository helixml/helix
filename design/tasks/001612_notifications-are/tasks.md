# Implementation Tasks

## Fix read state (acknowledge on click, not on open)

- [ ] In `GlobalNotifications.tsx`, remove the auto-acknowledge `for` loop from `handleDrawerOpen` (lines 243–248) and remove `events` and `acknowledge` from its `useCallback` dependency array
- [ ] In `handleNavigate`, call `acknowledge(event.id)` before navigating — so clicking a notification marks it as read

## Notification grouping

- [ ] Add a `groupEvents()` pure function in `GlobalNotifications.tsx` that pairs `specs_pushed` + `agent_interaction_completed` events for the same `spec_task_id` within 60 seconds into a `{ kind: 'grouped', primary, secondary }` group
- [ ] Update `AttentionEventItem` to accept an optional `groupedWith` prop (the secondary `AttentionEvent`); when present, show combined label ("📋 Spec ready & agent finished"), and have dismiss/navigate handlers operate on both events
- [ ] In the panel render, replace the raw `events.map(...)` with `groupEvents(events).map(...)` using the updated item component
- [ ] When navigating a grouped item, acknowledge both underlying events before navigating
- [ ] When dismissing a grouped item (X button), dismiss both underlying event IDs
