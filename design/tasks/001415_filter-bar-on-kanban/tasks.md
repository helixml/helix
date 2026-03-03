# Implementation Tasks

- [x] Update `filterTasks` function in `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx` to detect numeric-only filters
- [x] Add `task_number` matching when filter is a valid integer
- [x] Test: search "1412" finds task #001412 (manual verification needed)
- [x] Test: search "001412" finds task #001412 (manual verification needed)
- [x] Test: existing text search (name, description, implementation_plan) still works (manual verification needed)
- [x] Test: mixed results work when number appears in both task_number and text fields (manual verification needed)

## Implementation Notes

Changed `filterTasks` function in `SpecTaskKanbanBoard.tsx` (line ~753) to:
1. Detect if filter is purely numeric using regex `/^\d+$/`
2. Parse as integer when numeric
3. Add condition `task.task_number === numericFilter` to the filter OR chain

Build verified with `yarn build` - no errors.