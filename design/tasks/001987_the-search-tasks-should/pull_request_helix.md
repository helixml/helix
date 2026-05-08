# Persist spec-task search in URL; show creation date in title tooltip

## Summary
The "Search tasks..." filter on the spec-task kanban board is now persisted in the URL as `?search=...`. Browser Back from a task detail returns to the kanban with the search term still in the input — no more retyping. Page refresh and shared links also restore the filtered view.

The hover tooltip on each task card's title now shows the task's creation date on its own line above the existing description/prompt text, giving temporal context without opening the task.

## Changes
- `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx`
  - Replaced the local `useState` for the search input with router-backed state. Initial value comes from `router.params.search`; changes are written back via `router.mergeParams({ search })` (debounced 250ms). Empty input strips the param via `replaceParams` so the URL stays clean.
  - Removed the unused `searchFilter` prop (and its `searchFilterProp` default) from `SpecTaskKanbanBoardProps` — no callers passed it.
- `frontend/src/components/tasks/TaskCard.tsx`
  - Added a small `formatCreatedAt` helper (native `Date.toLocaleString` — codebase already eschews date-fns) that returns a formatted string or `null` for invalid input.
  - Title tooltip prepends `Created <date>\n\n` to the existing body when `created_at` is valid; otherwise falls back to the prior behaviour.

## Notes
- Uses router5's `mergeParams` / `replaceParams`, not raw `window.history.replaceState` — bypassing router5 corrupts the back-stack (documented at `frontend/src/hooks/useUrlTab.ts:14`).
- `router.tsx` runs router5 with `queryParamsMode: 'loose'`, so `?search=` works without declaring the param in the route path.
- Other filters (label, assignee) are still localStorage-backed; only the search filter is in scope per the user request.

## Screenshots
![Tooltip with creation date](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001987_the-search-tasks-should/screenshots/01-tooltip-with-date.png)
![Search term persisted in URL](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001987_the-search-tasks-should/screenshots/02-search-in-url.png)

## Test plan
- [x] `yarn build` (no TS errors)
- [x] Type in "Search tasks..." → URL gains `?search=...`
- [x] Reload → input pre-filled, list filtered
- [x] Click task → Back → input still has the search term
- [x] Click X clear-adornment → `?search=` dropped, other params preserved
- [x] Hover task title → tooltip shows creation date above the prompt
