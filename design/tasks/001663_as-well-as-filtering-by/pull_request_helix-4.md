# Add assignee filter to Kanban board

## Summary
Adds an assignee filter to the Kanban board header, alongside the existing label filter. Users can filter tasks by one or more assignees (OR semantics) or by "Unassigned". Selection persists across page refreshes via localStorage.

## Changes
- Added `assigneeFilter` state with localStorage persistence (keyed per project) in `SpecTaskKanbanBoard.tsx`
- Derived `availableAssigneeIds` from loaded tasks (only shows assignees that appear on the board)
- Extended `filteredTasks` useMemo to apply assignee filter with OR semantics after label filtering
- Added `Autocomplete` in the board header with member avatars + names; "Unassigned" option for tasks with no assignee
- No backend changes required — `assignee_id` already returned on every task, org members already available via `useAccount()`
