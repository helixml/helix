# Requirements: Filter by Task Number in Split Screen Plus Menu

## User Story

As a user in the split screen view, when I click the "+" button to open a task, I want to be able to type a task number (e.g. `1563`) in the search box and have it find the matching task — just like the main task list already supports.

## Acceptance Criteria

- [ ] Typing a pure number (e.g. `1563`) in the "+" search box in split screen view returns the task with that task number
- [ ] Typing a number that partially appears in a title (e.g. `15`) still matches by title substring as before
- [ ] Non-numeric queries continue to filter by title as before
- [ ] Task numbers displayed in the dropdown match the format shown in task headers (`#001563`)

## Current Behavior

The search in `TabsView.tsx` only matches task titles (via `user_short_title`, `short_title`, or `name`). Typing a task number like `1563` returns no results even if the task exists.

## Expected Behavior

Same numeric filtering logic as `SpecTaskKanbanBoard.tsx`: if the query is purely numeric, also match by `task.task_number`.
