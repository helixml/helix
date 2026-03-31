# Implementation Tasks

- [~] Add `assigneeFilter` state with localStorage persistence (keyed by `helix-assignee-filter-${projectId}`) in `SpecTaskKanbanBoard.tsx`
- [ ] Derive `availableAssignees` (list of assignee IDs present on loaded tasks, including `"__unassigned__"`) via `useMemo`
- [ ] Extend the `filteredTasks` `useMemo` to apply assignee filter with OR semantics after label filtering
- [ ] Add assignee filter `Autocomplete` to the board header, next to the label filter, rendering member avatars + names as options (use org members list already in scope)
- [ ] Handle `"__unassigned__"` option in the Autocomplete with an "Unassigned" label and appropriate icon
- [ ] Persist assignee filter selection to localStorage on change
