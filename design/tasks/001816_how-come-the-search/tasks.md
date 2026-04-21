# Implementation Tasks

- [~] Create `frontend/src/utils/searchUtils.ts` with a `matchesAllTokens(query, ...fields)` utility that splits on whitespace and requires all tokens to match
- [ ] Update `filterTasks` in `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx` (line 826) to use `matchesAllTokens` instead of `.includes()`
- [ ] Update the search filter in `frontend/src/components/tasks/BacklogTableView.tsx` (line 87) to use `matchesAllTokens` instead of `.includes()`
- [ ] Verify `yarn build` passes with no type errors
- [ ] Test in browser: search "github public" matches a task containing both words separately; single-word and numeric searches still work
