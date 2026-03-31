# Requirements: Dismiss All Matching Notifications

## Problem

When a user dismisses a notification, the notification reappears as bold (unread). This happens because:

1. Multiple `AttentionEvent` rows exist in the DB for the same `spec_task_id` (different event types or different commit qualifiers).
2. The API returns only the newest undismissed event per task via `DISTINCT ON (spec_task_id)`.
3. The dismiss action only sets `dismissed_at` on the single event ID passed, leaving older events undismissed.
4. After the query cache invalidates, the API returns the next-oldest event for that task, which is still undismissed — making the notification reappear bold.

## User Stories

- As a user, when I dismiss a notification, I expect it to be gone permanently, not reappear on next refresh.
- As a user, dismissing a notification for a task should dismiss all notifications for that task, not just the currently displayed one.

## Acceptance Criteria

- [ ] Dismissing a notification by event ID causes ALL events sharing the same `spec_task_id` (and `user_id`) to be marked dismissed.
- [ ] After dismissal, the notification does not reappear on page refresh or query invalidation.
- [ ] "Dismiss all" bulk operation is unaffected (already dismisses everything).
- [ ] The dismiss action remains scoped to the current user (no cross-user side effects).
