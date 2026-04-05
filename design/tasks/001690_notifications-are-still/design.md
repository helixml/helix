# Design: Notification Fixes

## Codebase Context

- **Notification hook:** `frontend/src/hooks/useAttentionEvents.ts` — single `useAttentionEvents(enabled, filterMine)` hook returns `events`, `newEvents`, `totalCount`, `unreadCount`, `hasNew`
- **Notification UI:** `frontend/src/components/system/GlobalNotifications.tsx` — bell icon, drawer panel, browser notification firing
- **Backend query:** `api/pkg/store/store_attention_events.go` — `ListAttentionEvents()` with `DISTINCT ON (spec_task_id)` deduplication
- **Event type (Go):** `api/pkg/types/attention_event.go` — `AttentionEvent` struct, currently has no assignee fields
- **Event type (TS):** `frontend/src/hooks/useAttentionEvents.ts:5-22` — `AttentionEvent` interface, no assignee fields
- **Avatar pattern:** `TaskCard.tsx:608-616` has `getAssigneeInitials()` and `TaskCard.tsx:1070-1101` renders 20×20px MUI `Avatar`

## Bug 1: Sort Order — FIXED in main

Already resolved. The query in `store_attention_events.go` now wraps in a subquery with `ORDER BY created_at DESC` on the outer query.

## Bug 2: Bell Icon Count Mismatch

**Root Cause:** `GlobalNotifications.tsx:495`:
```typescript
badgeContent={hasNew ? unreadCount : totalCount}
```
When unread events exist (`hasNew=true`), badge shows `unreadCount` while "Needs Attention" header shows `totalCount`. These diverge when some events are acknowledged but not dismissed.

**Fix:** Always show `totalCount`:
```typescript
badgeContent={totalCount}
```
The badge color already switches red/gray based on `hasNew`, which is sufficient to indicate new events.

## Bug 3: Browser Notifications + Mine/All — Already Working

Verified in code: `newEvents` is computed from `query.data` in `useAttentionEvents.ts:55-61`, which uses the `filterMine`-parameterized API URL. The `shownRef` deduplication in `useBrowserNotifications.ts` correctly prevents re-firing on filter toggle (desired behavior). No fix needed.

## Feature: User Avatar in "All" Mode

### Backend Changes

**Problem:** `AttentionEvent` struct has no assignee data. The `ListAttentionEvents` query does `SELECT *` from `attention_events` only — no JOIN to `spec_tasks`.

**Solution:** Add denormalized `assignee_name` field to `AttentionEvent` (same pattern as existing `project_name` and `spec_task_name` denormalization). Populate it at event creation time from the spec task's assignee.

1. Add to Go struct (`attention_event.go`):
```go
AssigneeName string `json:"assignee_name,omitempty" gorm:"size:255"`
```

2. In the attention service (`attention_service.go`), when creating events, look up the spec task's assignee and set `AssigneeName`. The spec task's `assignee_id` can be resolved to a name via the users table.

3. Add GORM auto-migration to pick up the new column.

**Why denormalize instead of JOIN?** Follows the existing pattern — `ProjectName` and `SpecTaskName` are already denormalized on the event struct for "display without joins" (see comment on line 29 of `attention_event.go`). Assignee name changes are rare enough that stale data is acceptable.

**Backfill:** Existing events in the DB won't have `assignee_name`. The frontend should handle empty `assignee_name` gracefully (no avatar shown). A one-time SQL backfill can be run if needed:
```sql
UPDATE attention_events ae
SET assignee_name = u.full_name
FROM spec_tasks st
JOIN users u ON u.id = st.assignee_id
WHERE ae.spec_task_id = st.id
  AND st.assignee_id IS NOT NULL
  AND st.assignee_id != '';
```

### Frontend Changes

1. Add `assignee_name?: string` to `AttentionEvent` interface in `useAttentionEvents.ts`

2. In `GlobalNotifications.tsx`, in the notification item render, add avatar to the left of the timestamp when `!filterMine`:

```tsx
{!filterMine && event.assignee_name && (
  <Avatar sx={{ width: 16, height: 16, fontSize: '0.5rem', mr: 0.5 }}>
    {getInitials(event.assignee_name)}
  </Avatar>
)}
```

3. `getInitials` helper (inline or extracted): split name, take first+last initial, uppercase. Same logic as `TaskCard.tsx:608-616`.

## Files to Change

| File | Change |
|------|--------|
| `api/pkg/types/attention_event.go` | Add `AssigneeName` field to struct |
| `api/pkg/services/attention_service.go` | Populate `AssigneeName` when creating events |
| `frontend/src/hooks/useAttentionEvents.ts` | Add `assignee_name` to TS interface |
| `frontend/src/components/system/GlobalNotifications.tsx` | Fix badge to use `totalCount`; add avatar in "all" mode |
