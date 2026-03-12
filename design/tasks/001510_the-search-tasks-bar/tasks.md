# Implementation Tasks

## Hook: `useSearchHistory`

- [ ] Create `frontend/src/hooks/useSearchHistory.ts` with the following API: `useSearchHistory(projectId: string, viewId: string)` → `{ history, addEntry, removeEntry, clearAll }`
- [ ] Implement localStorage read/write with key `helix_task_search_history_${projectId}_${viewId}`, storing a `string[]` as JSON
- [ ] Wrap `localStorage.setItem` in try/catch (matches existing pattern in `usePromptHistory.ts`)
- [ ] No-op when `projectId` is falsy (returns empty history, ignores adds)
- [ ] Cap history at 10 entries, evict oldest when full
- [ ] Deduplicate: adding an existing entry moves it to the top instead of creating a duplicate
- [ ] Never record empty or whitespace-only strings

## Component: `SearchHistoryDropdown`

- [ ] Create `frontend/src/components/tasks/SearchHistoryDropdown.tsx` — a presentational component that renders a list of history entries below a search TextField
- [ ] Use MUI `Popper` + `Paper` anchored to the TextField's container element
- [ ] Each row displays the search text and an X (`Clear` icon) button to remove that entry
- [ ] Clicking a row calls an `onSelect(entry)` callback and closes the dropdown
- [ ] Clicking X calls `onRemove(entry)` and keeps the dropdown open
- [ ] When the search field has typed text, filter displayed history entries to those that include the typed text (case-insensitive)
- [ ] Close on Escape keypress or click outside (MUI Popper's `ClickAwayListener`)
- [ ] Set z-index to `theme.zIndex.modal - 1` so it appears above Kanban board but below modals

## Integration: Kanban board (`SpecTaskKanbanBoard.tsx`)

- [ ] Import `useSearchHistory` and `SearchHistoryDropdown`
- [ ] Call `useSearchHistory(projectId, "kanban")` alongside existing `searchFilter` state
- [ ] Attach a ref to the search `TextField` (~L1339) to anchor the Popper
- [ ] Show `SearchHistoryDropdown` on focus when field is empty or text matches history entries
- [ ] Call `addEntry(searchFilter)` on Enter keypress
- [ ] Call `addEntry(searchFilter)` after 1 second debounce of no typing (only if field is non-empty)
- [ ] When user selects a history entry, set `searchFilter` to that value

## Integration: Backlog filter bar (`BacklogFilterBar.tsx`)

- [ ] Add `projectId` prop to `BacklogFilterBarProps` (passed down from `BacklogTableView`)
- [ ] Import `useSearchHistory` and `SearchHistoryDropdown`
- [ ] Call `useSearchHistory(projectId, "backlog")`
- [ ] Attach the same focus/Enter/debounce/select behavior as the Kanban integration
- [ ] Pass `projectId` from `BacklogTableView.tsx` (or its parent) into `BacklogFilterBar`

## Testing & verification

- [ ] Verify history persists across page reloads (manually or with a quick test)
- [ ] Verify separate projects have independent search histories
- [ ] Verify Kanban and Backlog maintain separate histories within the same project
- [ ] Verify the X button removes a single entry without closing the dropdown
- [ ] Verify duplicate searches get moved to top, not duplicated
- [ ] Run `cd frontend && yarn build` to confirm no build errors