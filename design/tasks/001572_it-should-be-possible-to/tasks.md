# Implementation Tasks

- [ ] In `store_spec_tasks.go` `ListSpecTasks`, update the ORDER BY clause to sort by priority (critical=4, high=3, medium=2, low=1) descending before `status_updated_at DESC`
- [ ] In `TaskCard.tsx`, add priority options (Critical, High, Medium, Low) to the "..." context menu, with a checkmark on the current value, calling `updateSpecTask` on click
- [ ] In `SpecTaskDetailContent.tsx`, make the priority chip always clickable (not gated by `isEditMode`), opening an anchored dropdown menu to change priority inline
- [ ] Verify that changing priority via menu or chip immediately reorders the task in the Kanban column after the next refetch (10s polling or manual invalidation)
