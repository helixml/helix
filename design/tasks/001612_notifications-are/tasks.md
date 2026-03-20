# Implementation Tasks

- [ ] In `frontend/src/components/system/GlobalNotifications.tsx`, remove the auto-acknowledge loop from `handleDrawerOpen` (delete lines 243–248: the `for` loop that calls `acknowledge(event.id)`)
- [ ] Update the `useCallback` dependency array for `handleDrawerOpen` — remove `events` and `acknowledge` since they are no longer referenced in the callback body
- [ ] Verify the red badge persists after opening the panel (badge count should not change on open)
- [ ] Verify notifications are still dismissed correctly via the X button and "Dismiss all"
