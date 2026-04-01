# Design: Fix Notifications Panel Ordering

## Root Cause

File: `api/pkg/store/store_attention_events.go`, line 92–101.

```sql
SELECT DISTINCT ON (spec_task_id) *
FROM attention_events
WHERE user_id = ?
  AND dismissed_at IS NULL
  AND (snoozed_until IS NULL OR snoozed_until < ?)
ORDER BY spec_task_id, created_at DESC
```

PostgreSQL's `DISTINCT ON` requires that the first `ORDER BY` column(s) match the `DISTINCT ON` expression. So the outer sort is by `spec_task_id` (a UUID string — random order), and `created_at DESC` only determines which row wins the deduplication within each group. The result set is **not** sorted by `created_at`.

The frontend comment at line 107 of `GlobalNotifications.tsx` incorrectly states:
> "Events are already sorted newest-first from the API, so the first group for each task is the most recent one."

This assumption is false. The API returns events in UUID order.

## Fix

### Backend (primary fix)

Wrap the deduplication in a subquery, then re-sort by `created_at DESC`:

```sql
SELECT * FROM (
  SELECT DISTINCT ON (spec_task_id) *
  FROM attention_events
  WHERE user_id = ?
    AND dismissed_at IS NULL
    AND (snoozed_until IS NULL OR snoozed_until < ?)
    -- orgFilter and mineFilter conditions go here too
  ORDER BY spec_task_id, created_at DESC
) AS deduped
ORDER BY created_at DESC
```

This is the minimal correct fix. The subquery handles deduplication; the outer `ORDER BY` handles presentation order.

### Frontend (defensive fix)

After `groupEvents()` returns groups, sort them by the most recent `created_at` in each group before rendering. This makes the frontend correct regardless of API response ordering:

```ts
function groupTimestamp(group: EventGroup): number {
  if (group.kind === 'grouped') {
    return Math.max(
      new Date(group.primary.created_at).getTime(),
      new Date(group.secondary.created_at).getTime(),
    )
  }
  return new Date(group.event.created_at).getTime()
}

// In GlobalNotifications, after groupEvents():
const groups = deduplicateGroupsByTask(groupEvents(events))
  .sort((a, b) => groupTimestamp(b) - groupTimestamp(a))
```

The same sort should be applied in the browser notification `useEffect` (line 357), though it doesn't affect display order.

## Files to Change

| File | Change |
|------|--------|
| `api/pkg/store/store_attention_events.go` | Wrap DISTINCT ON query in subquery; add outer `ORDER BY created_at DESC` |
| `frontend/src/components/system/GlobalNotifications.tsx` | Sort groups by timestamp after `deduplicateGroupsByTask`; fix incorrect comment |

## Notes

- The `mineFilter` subquery adds parameters (`userID, userID`) after `orgFilter` args. The raw SQL approach using `args` slice must maintain parameter order when wrapping in a subquery. No change to parameter order is needed — the subquery just adds a surrounding `SELECT * FROM (...) AS deduped ORDER BY created_at DESC`.
- No migration needed. No API changes. No new dependencies.
- The frontend fix is defensive belt-and-suspenders; the backend fix is the actual root cause resolution.
