# Design: Task Search History

## Overview

Add a search history dropdown to the Kanban board and Backlog table task search bars. History is stored in `localStorage` per project and shown when the user focuses an empty search field.

## Codebase Context

### Existing search bars (all `useState`-only, no persistence)

| Location | Component | State variable | Notes |
|----------|-----------|---------------|-------|
| Kanban toolbar | `SpecTaskKanbanBoard.tsx` ~L1339 | `searchFilter` | `TextField` with clear button |
| Backlog filter | `BacklogFilterBar.tsx` ~L62 | `search` (prop) | `TextField`, parent owns state |
| Tab picker menu | `TabsView.tsx` ~L1218 | `taskSearchQuery` | Ephemeral dropdown — **excluded** from this feature |

### Existing patterns to follow

- **`usePromptHistory` hook** (`hooks/usePromptHistory.ts`): Uses `localStorage` with a `helix_prompt_history` key prefix, debounced saves, max size cap. We'll use the same localStorage pattern but much simpler — search history is just a string array, not the complex entry objects that prompt history uses.
- **Workspace state persistence** (`TabsView.tsx` ~L1545): Uses `helix_workspace_state_${projectId}` key pattern for per-project localStorage. We'll follow the same project-scoped key convention.

## Architecture

### New hook: `useSearchHistory`

A single custom hook in `frontend/src/hooks/useSearchHistory.ts` that encapsulates all history logic.

```
useSearchHistory(projectId: string, viewId: string)
  → { history, addEntry, removeEntry, clearAll }
```

- **Storage key**: `helix_task_search_history_${projectId}_${viewId}` (viewId is `"kanban"` or `"backlog"` — separate histories since the two views serve different purposes)
- **Max entries**: 10 (oldest evicted when full)
- **Dedup**: Adding an existing entry moves it to the top
- **Data format**: `string[]` serialized as JSON

### New component: `SearchHistoryDropdown`

A small presentational component in `frontend/src/components/tasks/SearchHistoryDropdown.tsx` that renders the history list below the search `TextField`.

**Behavior:**
- Appears on focus when search field is empty OR when typed text matches history entries
- Each row: search text + X (clear) icon button
- Click row → fills search field, triggers filter, closes dropdown
- Click X → removes entry, dropdown stays open
- Escape or outside click → closes dropdown
- Uses MUI `Popper` + `Paper` anchored to the TextField (same pattern as autocomplete dropdowns elsewhere in the app)

### Integration points

1. **`SpecTaskKanbanBoard.tsx`**: Wrap the existing `TextField` (~L1339) — add `useSearchHistory("kanban")`, attach `SearchHistoryDropdown`, and call `addEntry` on Enter keypress or after 1s debounce of no typing.
2. **`BacklogFilterBar.tsx`**: Same approach for the search `TextField` (~L62). The `onSearchChange` prop still works as before; history is managed internally by the filter bar.

### When to record a search

A search is added to history when the user:
- Presses Enter in the search field, OR
- Stops typing for 1 second and the field is non-empty (debounce)

Empty/whitespace-only strings are never recorded.

## Key Decisions

| Decision | Rationale |
|----------|-----------|
| `localStorage` only, no backend | Search history is a convenience feature, not critical data. Keeps it simple. Matches existing `usePromptHistory` and workspace state patterns. |
| Per-project scoping | Projects have different task sets. Searching "auth" in project A is irrelevant in project B. Follows `helix_workspace_state_${projectId}` convention. |
| Separate Kanban/Backlog histories | The two views filter on different fields (Kanban searches name+description+plan, Backlog searches prompt text). A search useful in one may not make sense in the other. |
| Exclude TabsView picker | That search is inside a transient `Menu` dropdown for picking a tab. Adding history there would clutter a tiny UI for minimal benefit. |
| 10-entry cap | Enough to be useful, small enough to scan at a glance. |
| MUI `Popper` for dropdown | Consistent with MUI patterns used elsewhere. Avoids pulling in a full `Autocomplete` component which would require restructuring the existing `TextField` setup. |
| Debounce + Enter to record | Recording on every keystroke would fill history with partial queries. Enter gives explicit intent; the 1s debounce catches users who type and wait for results without pressing Enter. |

## Risks / Gotchas

- **`projectId` may be undefined** during initial load. The hook should no-op (return empty history, ignore adds) when projectId is falsy.
- **localStorage quota**: Not a real concern — 10 short strings per project is negligible. But wrap `setItem` in try/catch as the existing hooks do.
- **Popper z-index**: The Kanban board has drag-and-drop overlays. The history dropdown needs a z-index above the board but below modals. Use MUI theme's `zIndex.modal - 1`.