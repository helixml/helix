# Implementation Tasks

- [ ] Update `filterTasks` function in `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx` to detect numeric-only filters
- [ ] Add `task_number` matching when filter is a valid integer
- [ ] Test: search "1412" finds task #001412
- [ ] Test: search "001412" finds task #001412
- [ ] Test: existing text search (name, description, implementation_plan) still works
- [ ] Test: mixed results work when number appears in both task_number and text fields