# Implementation Tasks

- [~] Add `searchFilter` state to `SpecTasksPage.tsx` (useState with empty string default)
- [~] Add search TextField to topbar in `SpecTasksPage.tsx` (before view mode toggle buttons)
  - Use `SearchIcon` from `@mui/icons-material/Search` as startAdornment
  - Add clear button (X icon) as endAdornment when filter is not empty
  - Style to match existing topbar elements (size="small")
- [ ] Add `searchFilter?: string` prop to `SpecTaskKanbanBoardProps` interface
- [ ] Pass `searchFilter` prop from `SpecTasksPage` to `SpecTaskKanbanBoard`
- [ ] Create `filterTasks` helper function in `SpecTaskKanbanBoard.tsx` that filters by `name`, `description`, and `implementation_plan`
- [ ] Add `filteredTasks` useMemo in `SpecTaskKanbanBoard` that applies the filter
- [ ] Update `columns` useMemo to use `filteredTasks` instead of `tasks`
- [ ] Add `searchFilter` prop to `DroppableColumn` component
- [ ] Add "No matching tasks" empty state in `DroppableColumn` when column is empty due to filtering
- [ ] Test: verify filter works across all columns (backlog, planning, review, in progress, merged)
- [ ] Test: verify clearing filter restores all tasks
- [ ] Test: verify filter persists when switching between kanban and workspace views