# Implementation Tasks

- [x] In `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx`, replace the local `useState(searchFilterProp)` at line ~669 with a router5-backed value: read initial from `router.params.search`, keep an internal state for the input's controlled value, and on change call `router.mergeParams({ search: value || undefined })` (debounced ~250ms)
- [x] Remove the unused `searchFilterProp` parameter (and its `searchFilter?: string` prop in the interface around line 286) if no callers pass it — confirm with a grep before deleting
- [x] In `frontend/src/components/tasks/TaskCard.tsx` around line 766-789, prepend a date line to the tooltip content: when `task.created_at` is a valid date, render `Created <formatted>\n\n` before the existing `task.description || task.name`; format via native `new Date(...).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })` (no date-fns)
- [x] Guard the date formatter against missing/invalid `created_at` so the tooltip falls back cleanly to the current behaviour
- [x] Manually verify in the browser at http://localhost:8080:
  - [x] Type in "Search tasks..." → URL gains `?search=...` and the input value matches
  - [x] Reload → input is pre-filled, list is filtered
  - [x] Click a task to open detail, then Back → search term still in input
  - [x] Clear the input via the X → `search` param is removed from the URL
  - [x] Hover a task title → tooltip shows `Created <date>` line above the prompt/description
  - [x] Guard for invalid `created_at`: helper returns `null` and tooltip falls through to body — verified by code path (`isNaN(d.getTime())` check). No reproducible UI scenario in the seeded data.
- [x] `cd frontend && yarn build` to confirm no TypeScript errors (built in helix-frontend-1 container, both passes succeeded)
- [x] Push the feature branch (PR is created later via the Helix UI per the spec-task workflow — do NOT run `gh pr create`)
