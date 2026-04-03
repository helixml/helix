# Implementation Tasks

- [ ] Fix sort order: wrap `DISTINCT ON` query in a subquery and add `ORDER BY created_at DESC` to the outer query in `store_attention_events.go`
- [ ] Fix bell badge count: change `badgeContent={hasNew ? unreadCount : totalCount}` to `badgeContent={totalCount}` in `GlobalNotifications.tsx`
- [ ] Verify `newEvents` in `useAttentionEvents.ts` is derived from the same filtered query data (not a separate unfiltered source); fix if it is not
- [ ] Check if `AttentionEvent` already includes `assignee_id` / `assignee_name`; if not, add these fields to the backend SQL SELECT and Go struct, and update the TypeScript type
- [ ] Add assignee avatar to the left of the timestamp in notification items when `filterMine=false`, using MUI `Avatar` at 16–20px with initials matching the TaskCard style
