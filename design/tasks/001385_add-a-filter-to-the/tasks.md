# Implementation Tasks

- [x] Add `searchFilter` state to `SpecTasksPage.tsx` (useState with empty string default)
- [x] Add search TextField to topbar in `SpecTasksPage.tsx` (before view mode toggle buttons)
  - Use `SearchIcon` from `@mui/icons-material/Search` as startAdornment
  - Add clear button (X icon) as endAdornment when filter is not empty
  - Style to match existing topbar elements (size="small")
- [x] Add `searchFilter?: string` prop to `SpecTaskKanbanBoardProps` interface
- [x] Pass `searchFilter` prop from `SpecTasksPage` to `SpecTaskKanbanBoard`
- [x] Create `filterTasks` helper function in `SpecTaskKanbanBoard.tsx` that filters by `name`, `description`, and `implementation_plan`
- [x] Add `filteredTasks` useMemo in `SpecTaskKanbanBoard` that applies the filter
- [x] Update `columns` useMemo to use `filteredTasks` instead of `tasks`
- [x] Add `searchFilter` prop to `DroppableColumn` component
- [x] Add "No matching tasks" empty state in `DroppableColumn` when column is empty due to filtering
- [x] Test: verify filter works across all columns (backlog, planning, review, in progress, merged)
- [x] Test: verify clearing filter restores all tasks
- [x] Test: verify filter persists when switching between kanban and workspace views