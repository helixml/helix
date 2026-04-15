# Implementation Tasks

- [ ] Update `filterTasks` in `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx` (line 826) to split the search query on whitespace and require all tokens to match (AND logic) across the concatenated searchable fields
- [ ] Update the search filter in `frontend/src/components/tasks/BacklogTableView.tsx` (line 87) with the same multi-word split-and-match logic
- [ ] Verify `yarn build` passes with no type errors
- [ ] Test in browser: search "github public" matches a task containing both words separately; single-word and numeric searches still work
