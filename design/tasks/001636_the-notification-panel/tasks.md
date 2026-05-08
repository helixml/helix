# Implementation Tasks

- [x] In `GlobalNotifications.tsx`, after computing `groups`, add a helper `isGroupUnread(group)` that returns true if any event in the group lacks `acknowledged_at`
- [x] Derive `deduplicatedTotalCount`, `deduplicatedUnreadCount`, `deduplicatedHasNew` from the de-duplicated `groups` array
- [x] Replace `hasNew ? unreadCount : totalCount` and related `hasNew` references in the `<Badge>` with the new de-duplicated values
- [x] Also update header pill, dismiss-all button, and empty-state check to use de-duplicated counts (consistency)
- [x] Remove now-unused `totalCount`, `unreadCount`, `hasNew` from `useAttentionEvents` destructuring
- [x] Verify build: `cd frontend && yarn build` passes cleanly
- [x] Confirmed: acknowledging a grouped notification already marks both primary and secondary events (existing behavior in lines 579–581 calls `acknowledge(group.secondary.id)` then `handleNavigate(ev)` which calls `acknowledge(group.primary.id)`)
