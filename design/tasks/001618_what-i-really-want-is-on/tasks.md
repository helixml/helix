# Implementation Tasks

- [ ] Create `frontend/src/hooks/useNavigationHistory.ts` — hook that subscribes to router5 route changes, derives a title from route name + params, stores entries in `localStorage` (key: `helix_nav_history`), deduplicates by URL (keep most recent), caps at 30 entries, and returns the current history array
- [ ] Create `frontend/src/components/tasks/NavigationHistoryButton.tsx` — MUI `IconButton` (`ArrowDropDownIcon`) with `Tooltip` ("Recent pages"), opens a `Menu` listing history entries as `MenuItem`s (icon + title), navigates via `router.navigate(routeName, params)` on click, shows "No history yet" when empty
- [ ] In `SpecTaskKanbanBoard.tsx`, import and render `<NavigationHistoryButton />` at the right end of the existing filter/search bar row (the row containing the search `TextField` and label `Autocomplete`)
- [ ] Verify that navigating between a spec task detail page and a design review page records both entries and they appear in the dropdown in correct order
- [ ] Verify deduplication: visiting the same page twice shows it only once (most recent position)
- [ ] Verify history survives a page refresh (localStorage persistence)
