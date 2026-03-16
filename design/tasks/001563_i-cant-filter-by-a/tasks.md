# Implementation Tasks

- [ ] In `frontend/src/components/tasks/TabsView.tsx`, update the `filteredTasks` useMemo (lines ~786–794) to detect purely numeric queries and match against `task.task_number`, following the same pattern as `SpecTaskKanbanBoard.tsx` lines 756–767
- [ ] Verify the fix by searching for a task number in the split screen "+" menu and confirming it appears
