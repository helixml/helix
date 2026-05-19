# Implementation Tasks: Restore Kanban Board Scroll Position on Return Navigation

- [ ] Create `frontend/src/components/tasks/kanbanScrollMemory.ts` exporting `getKanbanScrollState`, `saveKanbanHorizontalScroll`, `saveKanbanColumnScroll`, `clearKanbanScrollState` backed by a module-scoped `Map<string, { horizontal: number; columns: Record<string, number> }>` keyed by `projectId`.
- [ ] In `SpecTaskKanbanBoard.tsx`, add a `useRef<HTMLDivElement>` for the outer column-strip `<Box>` (`SpecTaskKanbanBoard.tsx:1711-1735`) and attach it.
- [ ] In `SpecTaskKanbanBoard.tsx`, add a `useRef<Map<string, HTMLDivElement>>` registry and a callback-ref factory `setColumnNode(columnId)` that writes the DOM node into the registry (and removes it on null).
- [ ] In `DroppableColumn`, accept the parent's column-ref callback as a prop and compose it with the existing dnd-kit `setNodeRef` (call both refs in a single inline callback ref); apply the composed ref to the scrollable column body at `SpecTaskKanbanBoard.tsx:567-591`.
- [ ] Add an rAF-throttled `onScroll` to the outer strip that calls `saveKanbanHorizontalScroll(projectId, e.currentTarget.scrollLeft)`. Skip on mobile (`isMobile`).
- [ ] Add an rAF-throttled `onScroll` to each column body that calls `saveKanbanColumnScroll(projectId, column.id, e.currentTarget.scrollTop)`. Pass the column id through from `DroppableColumn`.
- [ ] In `SpecTaskKanbanBoard.tsx`, add a `hasRestoredRef = useRef(false)` and a `userHasScrolledRef = useRef(false)`; flip `userHasScrolledRef` on the **first** `onScroll` whose `event.isTrusted === true` *before* `hasRestoredRef.current` is set.
- [ ] Add a `useLayoutEffect` whose deps are `[projectId, loading, columns.length]` that: bails if `hasRestoredRef.current`, bails if `userHasScrolledRef.current`, bails if `loading || columns.length === 0`, reads the saved state for `projectId`, applies clamped `scrollLeft` to the outer strip (desktop only), iterates column refs applying clamped `scrollTop` per id, then sets `hasRestoredRef.current = true`.
- [ ] In the restoration effect, guard against `scrollWidth === 0` (board hidden behind Paywall, etc.) by re-scheduling once via `requestAnimationFrame` up to a small bounded retry count; do not loop indefinitely.
- [ ] Verify the mobile single-column path (`SpecTaskKanbanBoard.tsx:1744-1779`) — vertical restore still applies per column id; skip horizontal restore.
- [ ] Manually test in the inner Helix at `http://localhost:8080`: register/login, create a project with many tasks, scroll horizontally + scroll one column vertically, click a task to open detail, browser-back, confirm both scroll positions restore. Repeat with a different project to confirm per-project scoping.
- [ ] Manually test the "don't fight the user" path: open the board fresh, immediately start scrolling while data is loading, confirm restoration is skipped.
- [ ] Manually test clamping: scroll far down a column, archive most tasks in that column, return — confirm scroll lands at the new max without throwing.
- [ ] Run `cd frontend && yarn build` to confirm no TS errors.
- [ ] Commit with conventional commit `feat(frontend): restore kanban scroll position on return navigation` and push.
