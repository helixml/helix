# Design: Display Email Instead of User ID for Task Author

## Current State

- `SpecTask.created_by` stores the user's ID string (e.g., `101042449378557415004`).
- `SpecTaskDetailContent.tsx:1312` renders `task.created_by` directly as text.
- The codebase already has a working pattern for resolving user IDs to display names via organization memberships (see `AssigneeSelector.tsx:74-77` and `TaskCard.tsx:547-553`).

## Approach: Resolve from Organization Members (Frontend-Only)

The organization memberships list (`account.organizationTools.organization?.memberships`) is already loaded in the task context. Each membership contains a full `TypesUser` object with `full_name`, `email`, and `username`.

**Pattern to follow** (from `AssigneeSelector.tsx`):
```typescript
const getDisplayName = (user: TypesUser | undefined): string => {
  if (!user) return 'Unknown User'
  return user.full_name || user.username || user.email || 'Unknown User'
}
```

### Changes

**`SpecTaskDetailContent.tsx`** (~line 1310-1313):
1. Get `orgMembers` from `useAccount()` (already available in parent context or importable).
2. Find the member whose `user_id` matches `task.created_by`.
3. Display `full_name || email` instead of the raw ID.

**Decision**: Frontend-only fix. No backend changes needed — the user info is already available from the org memberships that are loaded for the assignee selector. This matches the existing pattern used by `TaskCard.tsx` for resolving assignee names.

### Codebase Patterns Found

- `TaskCard.tsx:548-553` — gets `orgMembers` from `useAccount()`, finds member by `user_id`
- `AssigneeSelector.tsx:74-77` — `getDisplayName()` with fallback chain
- `SpecTaskKanbanBoard.tsx:863` — same `orgMembers` access pattern

### Edge Cases

- User not found in org members (e.g., deleted user or cross-org task): show truncated ID or "Unknown User"
- Single-user orgs: still works, user is in their own org membership list
