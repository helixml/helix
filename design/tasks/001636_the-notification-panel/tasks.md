# Implementation Tasks

- [ ] In `GlobalNotifications.tsx`, after computing `groups`, add a helper `isGroupUnread(group)` that returns true if any event in the group lacks `acknowledged_at`
- [ ] Derive `deduplicatedTotalCount`, `deduplicatedUnreadCount`, `deduplicatedHasNew` from the de-duplicated `groups` array
- [ ] Replace `hasNew ? unreadCount : totalCount` and related `hasNew` references in the `<Badge>` with the new de-duplicated values
- [ ] Verify that acknowledging a grouped notification still marks both primary and secondary events (should be no change needed — existing behavior is correct)
- [ ] Manual test: create two events for the same task that get grouped → confirm badge shows "1" not "2"
