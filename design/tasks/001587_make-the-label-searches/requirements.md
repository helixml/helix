# Requirements: Persist Label Filter in Local Storage

## User Story

As a user filtering spec tasks by label, I want my label selections to persist across page
navigations and refreshes, so I don't have to reapply the same filters every time I return
to a project.

## Acceptance Criteria

1. When a user selects one or more labels in the label filter on the spec tasks page, those
   selections are saved to localStorage keyed by the current project ID.
2. When the user returns to the same project's spec tasks page, the previously selected labels
   are automatically restored from localStorage.
3. Different projects maintain independent label filter state (keyed by project ID).
4. Clearing the label filter removes the persisted state for that project.
5. The priority filter and search text are NOT persisted (out of scope).

## Scope

- **In scope:** `labelFilter` state in `BacklogTableView.tsx`
- **Out of scope:** priority filter, search text filter, Kanban board filter state

## Notes

- Project ID is available in `SpecTasksPage` via `router.params.id`
- `BacklogTableView` currently manages its own `labelFilter` state without persistence
- localStorage key pattern used elsewhere: `helix-<feature>` (no TTL needed here)
