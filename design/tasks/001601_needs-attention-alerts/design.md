# Design: "Needs Attention" Mine/Everyone Toggle

## Architecture

Filtering is done **server-side** via a new query parameter on the existing `GET /api/v1/attention-events` endpoint. This avoids shipping spec task creator data to the frontend on every poll and keeps the filter logic easy to extend for the assignee feature.

### Backend

**File:** `api/pkg/server/attention_event_handlers.go`

Add optional query param `filter=mine` to `listAttentionEvents`. When present, delegate to a modified store call.

**File:** `api/pkg/store/store_attention_events.go`

Add an optional `createdBy` parameter to `ListAttentionEvents`. When non-empty, join `spec_tasks` and restrict to rows where `spec_tasks.created_by = createdBy`:

```sql
SELECT ae.*
FROM   attention_events ae
JOIN   spec_tasks st ON st.id = ae.spec_task_id
WHERE  ae.user_id        = $userID
  AND  ae.dismissed_at   IS NULL
  AND  (ae.snoozed_until IS NULL OR ae.snoozed_until < NOW())
  AND  st.created_by     = $userID   -- "mine" filter
ORDER  BY ae.created_at DESC
```

**Future assignee support** (add later, no schema change needed):

```sql
  AND (
    st.assignee_id = $userID
    OR (st.assignee_id IS NULL AND st.created_by = $userID)
  )
```

No database migration is needed — `spec_tasks.created_by` already exists.

The store interface (`Store`) needs its `ListAttentionEvents` signature updated to accept a `filters` struct (or a `createdByFilter string` param). Prefer a small struct to keep the signature extensible:

```go
type AttentionEventFilters struct {
    CreatedBy string // empty = no filter (show all)
}
```

### Frontend

**File:** `frontend/src/hooks/useAttentionEvents.ts`

- Accept a `filterMine bool` param.
- Append `&filter=mine` to the query URL when true.
- Include `filterMine` in the React Query key so the two modes cache independently: `['attention-events', filterMine]`.

**File:** `frontend/src/components/system/GlobalNotifications.tsx`

- Add a small pill toggle **"Mine | All"** in the panel header, next to the title.
- Store preference in `localStorage` under key `attention-filter-mode` (`'mine' | 'all'`), default `'all'`.
- Pass the mode to `useAttentionEvents`.
- Badge count on the bell icon reflects the count from whichever mode is active.

### No Swagger/OpenAPI regeneration required

The new query param is optional and additive. The frontend uses `api.get()` directly (not the generated client) for attention events, so no `update_openapi` run is needed.

## Decisions

- **Server-side over client-side filtering**: avoids leaking spec task metadata into every event payload; cleaner extension path for assignee.
- **Default to "All"**: preserves existing behaviour; users opt in to the focused view.
- **localStorage persistence**: lightweight, no backend preference storage needed.
- **Badge reflects active filter**: the bell badge count matches what the user sees in the panel — no surprise counts when in "Mine" mode.
