# Fix: dismiss all events for a task when dismissing a notification

## Summary

Dismissing a notification was only marking the specific event ID as dismissed, leaving older events for the same task undismissed. Because the list API uses `DISTINCT ON (spec_task_id)` to deduplicate, the next-oldest undismissed event would surface on the next fetch — making the dismissed notification reappear as bold/unread.

## Changes

- `api/pkg/store/store_attention_events.go`: In `UpdateAttentionEvent()`, when `dismiss: true`, look up the `spec_task_id` for the given event ID, then set `dismissed_at` on **all** events with that `spec_task_id` + `user_id` instead of just the one event by ID.

## Testing

Inserted two `AttentionEvent` rows with the same `spec_task_id` (simulating two spec pushes). Dismissed the one returned by the API. Verified via DB query that both rows got `dismissed_at` set, and that a subsequent `GET /api/v1/attention-events` returned zero events.
