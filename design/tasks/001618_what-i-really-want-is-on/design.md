# Design: Navigation History Dropdown

## Architecture

Purely client-side. No backend changes. Three pieces:

1. **`useNavigationHistory` hook** — tracks route changes and persists history to `localStorage`
2. **`NavigationHistoryButton` component** — the icon button + MUI Menu dropdown
3. **Integration point** — added to the right side of `SpecTaskKanbanBoard.tsx`'s search/filter header bar

## Key Decisions

**Where to place it:** Inside `SpecTaskKanbanBoard.tsx` (not `SpecTasksPage.tsx`), in the existing filter/search bar row (the row with the search TextField and label Autocomplete), right-aligned. This puts it visually "just above the columns" as requested.

**Storage:** `localStorage` key `helix_nav_history`. Stores a JSON array of `{ url: string, title: string, timestamp: number }` objects, deduplicated by URL, capped at 30 entries, sorted newest-first.

**History tracking:** Use router5's route subscription (via the existing `useRouter` hook + `router.router.subscribe()`) to record each navigation. The current route's URL is reconstructed from `router.buildUrl(state.name, state.params)` or read from `window.location.pathname + window.location.search`.

**Page titles:** Derive human-readable titles from the route name and params:
- `org_project-spec-detail` → `"Task: {task title or task ID}"`
- `org_project-specs` → `"Board: {project name}"`
- `org_spec-task-review` / similar design-review routes → `"Review: {identifier}"`
- Fallback: use route name prettified

**Icon:** MUI `ArrowDropDownIcon` in an `IconButton`, with a `Tooltip` saying "Recent pages". Matches the style of other icon buttons in the toolbar.

**Dropdown UI:** MUI `Menu` anchored to the button. Each entry is a `MenuItem` with a small icon indicating type (e.g., `AssignmentIcon` for tasks, `RateReviewIcon` for reviews) and truncated title text. Empty state: "No history yet" (disabled `MenuItem`).

## File Changes

| File | Change |
|------|--------|
| `frontend/src/hooks/useNavigationHistory.ts` | New hook (create) |
| `frontend/src/components/tasks/NavigationHistoryButton.tsx` | New component (create) |
| `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx` | Add `NavigationHistoryButton` to the filter bar row |

## Data Shape

```ts
interface NavHistoryEntry {
  url: string;       // full pathname+search, e.g. "/orgs/123/projects/456/specs/789"
  title: string;     // human-readable label
  timestamp: number; // Date.now() at time of visit
}
```

## Patterns Used in This Codebase

- MUI `IconButton` + `Tooltip` + `Menu`/`MenuItem` for icon dropdowns — see the existing "more" menu in `SpecTasksPage.tsx`
- `useRouter()` from `src/hooks/useRouter.ts` for router access
- Route navigation: `router.navigate(routeName, params)` — but for arbitrary URLs, use `window.location.href` or parse stored URLs back to route names. Simplest: store full URL string and navigate via `window.location.href = url` (avoids needing to reverse-parse router5 routes), OR store route name+params for clean SPA navigation.
- **Recommended:** Store `{ routeName, params }` alongside the URL so navigation uses `router.navigate(routeName, params)` — cleaner and avoids full page reload.

## Revised Data Shape

```ts
interface NavHistoryEntry {
  url: string;           // for dedup key and display
  routeName: string;     // router5 route name
  params: Record<string, string>; // router5 params
  title: string;
  timestamp: number;
}
```
