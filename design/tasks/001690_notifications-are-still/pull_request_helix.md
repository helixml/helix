# Fix notification bell count mismatch and add assignee avatars

## Summary
Fixes the bell icon showing a different count than "Needs Attention" and adds assignee avatars to notification items when viewing all notifications.

## Changes
- Fix bell badge to always show `totalCount` instead of `unreadCount` when unread events exist, matching the "Needs Attention" header count
- Add `AssigneeName` denormalized field to `AttentionEvent` Go struct, populated at event creation time from the task's assignee
- Add `assignee_name` to the TypeScript `AttentionEvent` interface
- Show a small avatar with user initials to the left of the timestamp in notification items when in "all" mode (hidden in "mine" mode)
