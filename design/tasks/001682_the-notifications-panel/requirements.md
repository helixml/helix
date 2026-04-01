# Requirements: Notifications Panel Timestamp Ordering

## Problem

The notifications panel displays events in an unpredictable order. New notifications do not appear at the top. The user's hypothesis is correct: the grouping/deduplication logic is the cause.

**Root cause:** The SQL query uses `DISTINCT ON (spec_task_id)` which forces PostgreSQL to `ORDER BY spec_task_id, created_at DESC`. After deduplication, results are sorted by `spec_task_id` (a UUID — effectively random), not by `created_at`. The frontend assumes the API returns events sorted newest-first, but it does not.

## User Stories

**As a user**, I want to see my most recent notifications at the top of the panel so I can immediately act on the latest activity.

**As a user**, I want the timestamp shown on each notification to reflect when it actually happened, and the list order to match that timestamp ordering.

## Acceptance Criteria

1. The notifications panel lists events sorted newest-first by `created_at`.
2. After a new event arrives (via 10-second poll), it appears at the top of the list.
3. Grouped events (`specs_pushed` + `agent_interaction_completed`) are sorted by the most recent event's `created_at` within the group.
4. The timestamp label (e.g. "5m ago") is always consistent with the visual position in the list.
