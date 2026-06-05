# Notification badge: count de-duplicated groups, not raw events

## Summary
The notification bell badge was showing the raw count of attention events from the API,
but the panel itself displays a de-duplicated list of grouped notifications (one per
`spec_task_id`, with `specs_pushed` + `agent_interaction_completed` events within 60s
collapsed into a single item). This produced a mismatch — the badge could read "3 unread"
while the user saw a single notification in the panel.

This PR derives the badge counts from the same de-duplicated `groups` array the panel
renders, so the number on the bell matches what the user actually sees.

## Changes
- `GlobalNotifications.tsx`: added `isGroupUnread(group)` helper that returns true if any
  underlying event in the group lacks `acknowledged_at`.
- Computed `deduplicatedTotalCount`, `deduplicatedUnreadCount`, `deduplicatedHasNew` from
  the de-duplicated `groups` array.
- Updated the bell `<Badge>`, the header pill, the "Dismiss all" button visibility, and the
  "All clear" empty-state check to use the de-duplicated values.
- Removed `totalCount`, `unreadCount`, `hasNew` from the `useAttentionEvents` destructuring
  in this component (no longer used here; the hook still exposes them for other callers).

## Notes
The user asked: "when we click on one to mark it as acknowledged, do we mark all of the
underlying entries as acknowledged?" — yes, that already happens. For grouped items the
click handler calls `acknowledge(group.secondary.id)` and then `handleNavigate(group.primary)`
which calls `acknowledge(group.primary.id)`. No change needed there.
