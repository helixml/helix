# Implementation Tasks: Restore Kanban Board Scroll Position on Return Navigation

- [x] Create `frontend/src/components/tasks/kanbanScrollMemory.ts` exporting `getKanbanScrollState`, `saveKanbanHorizontalScroll`, `saveKanbanColumnScroll`, `clearKanbanScrollState` backed by a module-scoped `Map<string, { horizontal: number; columns: Record<string, number> }>` keyed by `projectId`.
- [x] In `SpecTaskKanbanBoard.tsx`, add a `useRef<HTMLDivElement>` for the outer column-strip `<Box>` and attach it.
- [x] In `SpecTaskKanbanBoard.tsx`, add a `useRef<Map<string, HTMLDivElement>>` registry and a callback-ref factory `getColumnRefSetter(columnId)` that writes the DOM node into the registry (and removes it on null). Memoized per id via a setter cache so identity is stable across renders.
- [x] In `DroppableColumn`, accept `columnBodyRef` + `onColumnScroll` props; replaced the existing no-op `setNodeRef` (the file's `setNodeRef` was already a stub — there is no real dnd-kit binding to compose with). Apply both to the scrollable column body Box.
- [x] Add an rAF-throttled `onScroll` to the outer strip that calls `saveKanbanHorizontalScroll(projectId, e.currentTarget.scrollLeft)`. Skip on mobile (`isMobile`).
- [x] Add an rAF-throttled `onScroll` to each column body that calls `saveKanbanColumnScroll(projectId, column.id, e.currentTarget.scrollTop)`. Column id flows from `makeColumnScrollHandler(columnId)`.
- [x] In `SpecTaskKanbanBoard.tsx`, add `hasRestoredRef`, `userHasScrolledRef`, and `isRestoringRef`; flip `userHasScrolledRef` on any scroll that arrives before restoration completes (and isn't itself the restoration write). Reset both restoration guards when `projectId` changes so switching projects mid-session works as a fresh mount.
- [x] Add a `useLayoutEffect` whose deps are `[projectId, loading, columns, isMobile]` that: bails if `hasRestoredRef`, marks restored+bails if `userHasScrolledRef`, bails if `loading || columns.length === 0`, reads the saved state for `projectId`, applies clamped `scrollLeft` to the outer strip (desktop only), iterates column refs applying clamped `scrollTop` per id, then sets `hasRestoredRef.current = true` and unsets `isRestoringRef` on the next animation frame.
- [x] In the restoration effect, guard against `scrollWidth === 0` (board hidden behind Paywall, etc.) by returning early without marking restored — the effect re-runs on the next column-content / loading change, so layout converges naturally.
- [x] Verify the mobile single-column path — vertical restore still applies per column id; horizontal restore + horizontal save are both gated on `!isMobile`.
- [x] Clean up pending rAF writes on unmount (mount cleanup effect).
- [x] Run `npx tsc --noEmit` to confirm no TS errors.
- [ ] Manually test in the inner Helix at `http://localhost:8080`: register/login, create a project with many tasks, scroll horizontally + scroll one column vertically, click a task to open detail, browser-back, confirm both scroll positions restore. Repeat with a different project to confirm per-project scoping.
- [ ] Manually test the "don't fight the user" path: open the board fresh, immediately start scrolling while data is loading, confirm restoration is skipped.
- [ ] Manually test clamping: scroll far down a column, archive most tasks in that column, return — confirm scroll lands at the new max without throwing.
- [ ] Commit with conventional commit `feat(frontend): restore kanban scroll position on return navigation` and push.

## Notes

- Discovered that the existing `setNodeRef` inside `DroppableColumn` was a no-op
  (`const setNodeRef = (node: HTMLElement | null) => {};`) — the inline comment
  "Simplified - no drag and drop, no complex interactions" confirms drag/drop
  was removed earlier. There was nothing to compose with, so we cleanly replace
  it with `columnBodyRef`.
- TS hoisting bug surfaced once: the restoration `useLayoutEffect` must be
  placed **after** `loading` is destructured from `useSpecTasks`, not next to
  the `columns` memo (which lives earlier in the file). Moved it to sit right
  after the `useSpecTasks` block to keep deps in scope.
- `useSpecTasks` polls every 3.1 s. Each poll updates `tasks` → `columns`, so
  `columns` identity changes frequently. The `hasRestoredRef` gate ensures we
  only restore once per mount despite this dep churn.
