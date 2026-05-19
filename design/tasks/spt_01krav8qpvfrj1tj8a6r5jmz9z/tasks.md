# Implementation Tasks: Restore Kanban Board Scroll Position on Return Navigation

- [x] Create `frontend/src/components/tasks/kanbanScrollMemory.ts` exporting `getKanbanScrollState`, `saveKanbanHorizontalScroll`, `saveKanbanColumnScroll`, `clearKanbanScrollState` backed by a module-scoped `Map<string, { horizontal: number; columns: Record<string, number> }>` keyed by `projectId`.
- [x] In `SpecTaskKanbanBoard.tsx`, add a `useRef<HTMLDivElement>` for the outer column-strip `<Box>` and attach it.
- [x] In `SpecTaskKanbanBoard.tsx`, add a `useRef<Map<string, HTMLDivElement>>` registry and a callback-ref factory `getColumnRefSetter(columnId)` that writes the DOM node into the registry (and removes it on null). Memoized per id via a setter cache so identity is stable across renders.
- [x] In `DroppableColumn`, accept `columnBodyRef` + `onColumnScroll` props; replaced the existing no-op `setNodeRef` stub. Apply both to the scrollable column body Box.
- [x] Add an `onScroll` to the outer strip that calls `saveKanbanHorizontalScroll(projectId, e.currentTarget.scrollLeft)`. Skip on mobile (`isMobile`). Synchronous save — see Notes.
- [x] Add an `onScroll` to each column body that calls `saveKanbanColumnScroll(projectId, column.id, e.currentTarget.scrollTop)`. Column id flows from `makeColumnScrollHandler(columnId)`.
- [x] In `SpecTaskKanbanBoard.tsx`, add `hasRestoredRef`, `userHasScrolledRef`, and `isRestoringRef`; flip `userHasScrolledRef` on any scroll that arrives before restoration completes (and isn't itself the restoration write). Reset both restoration guards when `projectId` changes so switching projects mid-session works as a fresh mount.
- [x] Add a `useLayoutEffect` whose deps are `[projectId, loading, columns, isMobile]` that: bails if `hasRestoredRef`, marks restored+bails if `userHasScrolledRef`, bails if `loading || columns.length === 0`, reads the saved state for `projectId`, applies clamped `scrollLeft` to the outer strip (desktop only), iterates column refs applying clamped `scrollTop` per id. **Re-runs until all targets satisfied** (distinguishes "data not loaded" from "column shrunk" via `columns.some(c => c.tasks.length > 0)`).
- [x] In the restoration effect, guard against `scrollWidth === 0` (board hidden behind Paywall, etc.) by returning early without marking restored — the effect re-runs on the next column-content / loading change, so layout converges naturally.
- [x] Verify the mobile single-column path — vertical restore still applies per column id; horizontal restore + horizontal save are both gated on `!isMobile`.
- [x] Run `npx tsc --noEmit` to confirm no TS errors.
- [x] Manually test in the inner Helix at `http://localhost:8080`: scrolled horizontally (200) + scrolled backlog column vertically (400), did SPA navigation away and back, both positions restored to (200, 400). Screenshots: `screenshots/02-kanban-after-restore.png`, `screenshots/03-kanban-restored-with-tasks.png`.
- [x] Manually test live-save: changed backlog scroll to 700 then navigated; on return, restored to 700 (latest value, not previous 400).
- [x] Manually test clamping: scrolled backlog to its max (2267), archived 12 tasks so the column shrank, navigated back — scrollTop landed at the new max (0) without throwing.
- [x] Commit with conventional commit `feat(frontend): restore kanban scroll position on return navigation` and push.

## Notes

- Discovered that the existing `setNodeRef` inside `DroppableColumn` was a no-op
  (`const setNodeRef = (node: HTMLElement | null) => {};`) — the inline comment
  "Simplified - no drag and drop, no complex interactions" confirms drag/drop
  was removed earlier. Nothing to compose with, so we cleanly replace it.
- TS hoisting bug surfaced once: the restoration `useLayoutEffect` must be
  placed **after** `loading` is destructured from `useSpecTasks`, not next to
  the `columns` memo. Moved it to sit right after the `useSpecTasks` block.
- `useSpecTasks` polls every 3.1 s. The `hasRestoredRef` gate prevents
  restoration from running again after initial success, despite `columns`
  identity changing on each poll.
- **Dropped the rAF throttling on save handlers.** Initial design used rAF
  to batch saves, but the cleanup effect cancelled pending rAFs on unmount —
  meaning the last scroll position before navigation was lost. Synchronous
  `Map.set` per scroll event is cheap and reliable.
- **Restore effect must re-run until satisfied.** On warm-cache remount,
  render 1 has `tasks=[]` (local state) even though `loading=false` (cache
  hit). Columns render empty bodies. The restore effect must defer (not mark
  hasRestored) until columns have actual height. To avoid an infinite-retry
  loop when a column has *genuinely* shrunk, gate "defer vs clamp" on
  `columns.some(c => c.tasks.length > 0)` — once any column has tasks, we
  trust the current state.
