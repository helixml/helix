# Fix notifications panel ordering: sort by newest-first

## Summary

The notifications panel displayed events in random order and new notifications did not appear at the top. The root cause was the SQL query using `DISTINCT ON (spec_task_id)`, which requires `ORDER BY spec_task_id, created_at DESC` — meaning PostgreSQL returned results sorted by `spec_task_id` (a UUID, effectively random), not by time.

## Changes

- **`api/pkg/store/store_attention_events.go`**: Wrap the `DISTINCT ON` deduplication query in a subquery and add `ORDER BY created_at DESC` on the outer query, so the API always returns events newest-first.

- **`frontend/src/components/system/GlobalNotifications.tsx`**: Add a `groupTimestamp()` helper that returns the most recent `created_at` for a group (accounting for paired events), and sort groups newest-first after `deduplicateGroupsByTask()`. Applied in both the render path and the browser notification `useEffect`. Also fix an incorrect comment that claimed the API returned events newest-first.
