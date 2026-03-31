# Implementation Tasks

- [x] Add `assigneeFilter` state with localStorage persistence (keyed by `helix-assignee-filter-${projectId}`) in `SpecTaskKanbanBoard.tsx`
- [x] Derive `availableAssigneeIds` (list of assignee IDs present on loaded tasks, including `"__unassigned__"`) via `useMemo`
- [x] Extend the `filteredTasks` `useMemo` to apply assignee filter with OR semantics after label filtering
- [x] Add assignee filter `Autocomplete` to the board header, next to the label filter, rendering member avatars + names as options (use org members list already in scope)
- [x] Handle `"__unassigned__"` option in the Autocomplete with an "Unassigned" label and appropriate icon
- [x] Persist assignee filter selection to localStorage on change
