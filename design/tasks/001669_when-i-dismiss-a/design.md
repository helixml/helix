# Design: Dismiss All Matching Notifications

## Root Cause

The store's `UpdateAttentionEvent()` dismisses by `WHERE id = ?`. But the API query (`ListAttentionEvents`) uses `DISTINCT ON (spec_task_id) ORDER BY created_at DESC`, meaning there can be multiple undismissed rows per task — only the newest surfaces in the UI. Dismissing the newest leaves older rows alive, which then surface on the next fetch.

## Fix: Dismiss by spec_task_id

Change the dismiss path in the store so it updates **all events for the same task + user**, not just the one by ID.

### Backend Change

**File**: `api/pkg/store/store_attention_events.go`

In `UpdateAttentionEvent()`, when processing a dismiss update, change the WHERE clause from:

```go
// Before (dismisses only one event)
Where("id = ?", id)
```

to a two-step approach:

```go
// Step 1: fetch the spec_task_id for the given event ID
var event types.AttentionEvent
s.gdb.WithContext(ctx).Select("spec_task_id", "user_id").Where("id = ?", id).First(&event)

// Step 2: dismiss all events for that task + user
s.gdb.WithContext(ctx).
    Model(&types.AttentionEvent{}).
    Where("spec_task_id = ? AND user_id = ?", event.SpecTaskID, event.UserID).
    Update("dismissed_at", &now)
```

The ownership check in the handler (`attention_event_handlers.go` line 100) already verifies the event belongs to the calling user before reaching the store, so `user_id` scoping is a safety double-check.

### No Frontend Changes Needed

The frontend already invalidates the query cache after dismissal (`onSuccess: invalidate` in `useAttentionEvents.ts:84`). Once the backend dismisses all matching events, the refetch will return nothing for that task.

## Key Files

- `api/pkg/store/store_attention_events.go` — `UpdateAttentionEvent()` function (~lines 113–144): change dismiss logic
- `api/pkg/server/attention_event_handlers.go` — handler, no change needed
- `frontend/src/hooks/useAttentionEvents.ts` — no change needed

## Decision: Store vs Handler

Considered handling this in the API handler (look up `spec_task_id` then call a new bulk-dismiss-by-task store method). Chose to keep it in the store because the store already has full DB access and co-locating the dismiss logic avoids an extra round-trip and keeps the handler thin.
