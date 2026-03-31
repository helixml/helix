# Sort current user to top of assignees list in spectask kanban

## Summary
When opening the assignee picker on a task card, the logged-in user now appears first in the list (below "Unassigned"), making self-assignment a single click.

## Changes
- `AssigneeSelector.tsx`: added optional `currentUserId` prop; sort logic in `filteredMembers` useMemo places the matching member first while preserving alphabetical order for everyone else
- `TaskCard.tsx`: passes `account.user?.id` as `currentUserId` to `AssigneeSelector`
