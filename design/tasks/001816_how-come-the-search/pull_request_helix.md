# Fix multi-word search in Kanban board and backlog table

## Summary
Multi-word searches like "github public" now match tasks containing all search terms anywhere in the text, instead of requiring the exact substring. Previously, the search treated the entire input as a literal substring match, which failed when the words appeared in different parts of the text.

## Changes
- Add shared `matchesAllTokens()` utility in `frontend/src/utils/searchUtils.ts` that splits queries on whitespace and requires all tokens to match (AND logic)
- Update `SpecTaskKanbanBoard.tsx` `filterTasks` to use `matchesAllTokens` instead of `.includes()`
- Update `BacklogTableView.tsx` search filter to use `matchesAllTokens` instead of `.includes()`

## Test results (browser console verification)
| Query | Expected | Result |
|-------|----------|--------|
| `"github public"` | true | true |
| `"GitHub public"` | true | true |
| `"public"` | true | true |
| `"private repos"` | true | true |
| `"nonexistent word"` | false | false |
| `""` (empty) | true | true |
| `"github   public"` (extra spaces) | true | true |
