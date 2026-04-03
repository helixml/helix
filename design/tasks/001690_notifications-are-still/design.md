# Design: Notification Fixes

## Bug 1: Sort Order

**Root Cause:** PostgreSQL `DISTINCT ON` with `ORDER BY spec_task_id, created_at DESC` deduplicates correctly but leaves rows ordered by `spec_task_id` (UUID), not by recency of the kept event.

**Fix:** Wrap the query in a subquery and add an outer `ORDER BY created_at DESC`:

```go
// store_attention_events.go
result := s.gdb.WithContext(ctx).Raw(`
  SELECT * FROM (
    SELECT DISTINCT ON (spec_task_id) *
    FROM attention_events
    WHERE user_id = ?
      AND dismissed_at IS NULL
      AND (snoozed_until IS NULL OR snoozed_until < ?)
      `+orgFilter+`
      `+mineFilter+`
    ORDER BY spec_task_id, created_at DESC
  ) AS deduped
  ORDER BY created_at DESC
`, args...).Scan(&events)
```

## Bug 2: Bell Icon Count Mismatch

**Root Cause:** `GlobalNotifications.tsx` badge logic:
```typescript
badgeContent={hasNew ? unreadCount : totalCount}
```
When any event is unread (`hasNew=true`), the badge shows `unreadCount` instead of `totalCount`, causing it to differ from the "Needs Attention" count in the panel.

**Fix:** Always show `totalCount`:
```typescript
badgeContent={totalCount}
```
The color already switches red/gray based on `hasNew`, which is sufficient visual indication of new events.

## Bug 3: Bell Count Respects Mine/All (Verification)

The hook `useAttentionEvents(true, filterMine)` is already called once in `GlobalNotifications.tsx` and both the badge and panel use results from this same call. The badge count mismatch (bug 2) was the only actual issue. No separate fix needed beyond bug 2.

## Bug 4: Browser Notifications and Mine/All

**Root Cause:** `newEvents` comes from `useAttentionEvents(true, filterMine)`, so it already respects the filter. However, the `shownRef` (a `Set<string>`) in `useBrowserNotifications` persists event IDs across filter switches. If an event was shown while in "all" mode, it won't re-fire when switching to "mine" even if it matches — this is correct behavior (don't spam re-fire on toggle).

The actual concern is: when `filterMine=true`, only mine events should produce browser notifications. This already works because `newEvents` is the filtered query result. No fix needed if `newEvents` correctly reflects the filter.

**Verify during implementation:** Confirm `newEvents` is derived from the same filtered `query.data` in `useAttentionEvents.ts`. If it uses a separate unfiltered source, fix it to use the filtered events.

## Bug 5: User Avatar in "All" Mode

**Data:** `AttentionEvent` needs to carry enough info to show the assignee. Check whether `assignee_id` / `assignee_name` is already on the event payload; if not, the backend must include it.

**Backend (`store_attention_events.go`):** Add a JOIN or include `assignee_id` and a display name field in the SQL SELECT. Alternatively, the frontend can look up `assignee_id` against `orgMembers` if `assignee_id` is already returned on the event.

**Frontend:** In the notification item render (inside `GlobalNotifications.tsx`), conditionally render an avatar to the left of the timestamp when `!filterMine`:

```tsx
{!filterMine && assigneeId && (
  <Avatar sx={{ width: 16, height: 16, fontSize: '0.5rem', mr: 0.5 }}>
    {getInitials(assigneeName)}
  </Avatar>
)}
<Typography variant="caption" sx={{ ... }}>
  {formatRelativeTime(event.created_at)}
</Typography>
```

**Avatar style:** Match `TaskCard.tsx` — MUI `Avatar`, 16–20px, initials from first+last letter of assignee's full_name, fallback to username/email.

**Pattern notes:** `getAssigneeInitials` in `TaskCard.tsx:608-616` and `getInitials` in `AssigneeSelector.tsx:80-88` both implement the same initials logic — extract to a shared util or replicate inline.

## Files to Change

| File | Change |
|------|--------|
| `api/pkg/store/store_attention_events.go` | Wrap in subquery, add outer `ORDER BY created_at DESC`; add assignee fields to SELECT |
| `frontend/src/components/system/GlobalNotifications.tsx` | Fix badge count to always use `totalCount`; add avatar to notification items in "all" mode |
| `frontend/src/hooks/useAttentionEvents.ts` | Verify `newEvents` uses filtered data; no other change expected |
| API type definitions (TypeScript) | Add `assignee_id`, `assignee_name` (or similar) to `AttentionEvent` if not present |
