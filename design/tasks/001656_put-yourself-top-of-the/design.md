# Design: Put Current User at Top of Assignees List

## Overview

Modify `AssigneeSelector.tsx` to sort the current user to the top of the filtered member list. The current user ID is already available as a prop (`currentUserId`) passed from `TaskCard.tsx`, which gets it from the account context via `account.user?.id`.

## Key Files

- `frontend/src/components/tasks/AssigneeSelector.tsx` — the only file that needs changing
- `frontend/src/components/tasks/TaskCard.tsx` — passes `currentUserId` to AssigneeSelector (check if prop already exists; if not, add it)

## Current Behavior

In `AssigneeSelector.tsx`, `filteredMembers` is derived by filtering `members` (org memberships) by search query. The list order comes from `useOrganizations`, which sorts alphabetically by name.

## Proposed Change

After filtering by search query, re-sort so the current user's membership entry is first:

```typescript
const filteredMembers = members
  .filter(/* existing search filter */)
  .sort((a, b) => {
    if (a.user_id === currentUserId) return -1;
    if (b.user_id === currentUserId) return 1;
    return 0; // preserve existing alphabetical order for others
  });
```

## Passing currentUserId

Check if `AssigneeSelector` already receives `currentUserId`. If not:
1. In `TaskCard.tsx`, add `currentUserId={account.user?.id}` to the `<AssigneeSelector>` call
2. Add `currentUserId?: string` to `AssigneeSelector`'s props interface

## Pattern Note

`ProjectMembersBar.tsx` already does this same "current user first" pattern (line 196-198). This task applies the same logic to the assignee picker.
