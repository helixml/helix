# Implementation Tasks

- [x] In `AssigneeSelector.tsx`, check if `currentUserId` prop already exists; if not, add it to the props interface
- [x] In `TaskCard.tsx`, pass `currentUserId={account.user?.id}` to `<AssigneeSelector>` if not already passed
- [x] In `AssigneeSelector.tsx`, after the search filter, sort `filteredMembers` so the entry matching `currentUserId` comes first
