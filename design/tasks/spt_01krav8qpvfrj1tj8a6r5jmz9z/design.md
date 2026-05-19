# Design: Restore Kanban Board Scroll Position on Return Navigation

## Summary

Persist the Kanban board's horizontal strip `scrollLeft` and each column's
`scrollTop` in an **in-memory, module-scoped map keyed by `projectId`**.
Save on every scroll (rAF-throttled). Restore once per mount, after task data
has loaded and the columns have laid out, using a `useLayoutEffect` gated on
`!loading && columns.some(c => c.tasks.length > 0)`. Bail out of restoration if
the user has scrolled in the meantime.

## Where the Scroll Containers Live

| Container | File | Element | Property |
|---|---|---|---|
| Outer column strip | `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx:1711-1735` | Currently anonymous `<Box>` | `scrollLeft` |
| Each column body | `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx:567-591` (`DroppableColumn`) | `<Box ref={setNodeRef}>` (a dnd-kit ref) | `scrollTop` |

The column body already has a ref consumed by `@dnd-kit`'s `useDroppable`.
We need a **second ref** on the same node — combine with a small
`mergeRefs(setNodeRef, ourRef)` helper (or use `useCallback` ref forwarding).

## Key Decisions

### D1 — In-memory store, not sessionStorage

The user request is about *jumping back when navigating within the SPA*, not
across reloads. An in-memory store:

- Has no quota concerns.
- Is automatically scoped to the current SPA session.
- Avoids the "I closed the tab two days ago and now it jumps somewhere weird"
  failure mode.

Implementation: a module-level `Map<string, KanbanScrollState>` in a new file
`frontend/src/components/tasks/kanbanScrollMemory.ts`. Keyed by
`projectId`. The shape:

```ts
type KanbanScrollState = {
  horizontal: number;            // scrollLeft of outer strip
  columns: Record<string, number>; // columnId -> scrollTop
};
```

Exported helpers (pure, no React):

```ts
export const getKanbanScrollState: (projectId: string) => KanbanScrollState | undefined;
export const saveKanbanHorizontalScroll: (projectId: string, scrollLeft: number) => void;
export const saveKanbanColumnScroll: (projectId: string, columnId: string, scrollTop: number) => void;
export const clearKanbanScrollState: (projectId: string) => void;
```

The map lives in module scope, so it survives `SpecTaskKanbanBoard` unmount
(which is exactly the scenario we're solving). It is cleared by garbage
collection on full reload.

### D2 — Save on scroll (rAF-throttled), not on unmount

We *could* save on unmount, but:

- React unmount ordering means the DOM may already be torn down when our
  cleanup runs, making `el.scrollLeft` unreliable.
- Saving live means the value survives even if the unmount path is weird
  (route guard, error boundary, etc.).

Pattern: attach an `onScroll` handler that schedules a `requestAnimationFrame`
write. Cancel the pending rAF if a new scroll arrives before it fires. This
keeps writes ≤ 60/s without lag.

### D3 — Restore in `useLayoutEffect`, gated on render readiness

The board mounts in three phases:

1. First paint: `loading = true`, columns empty.
2. React Query resolves: `loading = false`, `columns` populated, DOM grows.
3. User interacts.

We want restoration to land between (2) and the user's first interaction. Use
`useLayoutEffect` so the scroll is applied **before** the browser paints — no
flash of `scrollTop = 0`. Gate it with a `hasRestoredRef` so it runs at most
once per mount.

The gate condition:

```ts
const ready = !loading && columns.length > 0 &&
  columns.some(c => c.tasks.length > 0);
```

(We also accept "all columns truly empty" as ready, but in that case there's
nothing to scroll to anyway, so we can skip.)

### D4 — Detect user-initiated scrolls to abort restoration

If task data takes 2 s to load and the user impatiently scrolls the empty
board, we must not yank them back. Track a `userHasScrolledRef = useRef(false)`
flipped to `true` on the **first** scroll event whose `event.isTrusted` is true
*and* that happens before `hasRestoredRef.current === true`. Once restoration
runs (or is skipped), we stop caring.

Alternative considered: dispatch our restoration with `behavior: "instant"` and
detect via `requestIdleCallback`. Rejected — `useLayoutEffect` is simpler and
deterministic.

### D5 — Clamp restored values to current container size

Between leaving and returning, the user may have created/archived tasks. The
old `scrollLeft = 800` may exceed the new `scrollWidth`. Apply:

```ts
el.scrollLeft = Math.min(saved.horizontal, el.scrollWidth - el.clientWidth);
```

Browsers already clamp silently for `scrollLeft`/`scrollTop` assignment, but
clamping explicitly keeps the *saved* value coherent with reality after the
first restore (so subsequent saves don't carry forward an out-of-range value).

### D6 — Mobile single-column view

`isMobile` renders one column at a time keyed by `mobileColumnIndex`. The
**horizontal** scroll position is meaningless here (single column, hidden
overflow). Skip horizontal save/restore on mobile, but still save/restore
each column's `scrollTop`. The mobile column index itself is *not* persisted
(out of scope — it's reset on mount today and that's fine).

### D7 — Don't fight `useSpecTasks` polling

`useSpecTasks` refetches every 3.1 s. Each refetch may slightly change column
heights as tasks update. We do **not** re-run restoration on each poll — the
`hasRestoredRef` gate ensures we only restore once. After the first restore,
the user's manual scroll is authoritative and gets persisted.

## Data Flow

```
User on Kanban, scrolls column 'planning' to scrollTop=420
  └─> onScroll handler
      └─> rAF schedules saveKanbanColumnScroll(projectId, 'planning', 420)
          └─> module-level Map updated

User clicks task card
  └─> orgNavigate('project-task-detail', { id: projectId, taskId })
      └─> SpecTaskKanbanBoard unmounts; Map persists

User clicks 'back'
  └─> SpecTaskKanbanBoard mounts; reads Map for projectId
      └─> useLayoutEffect waits for data
          └─> when ready & !userHasScrolled: applies scrollLeft + per-column scrollTop
              └─> hasRestoredRef = true; subsequent scrolls are user-driven saves
```

## Files Touched

| File | Change |
|---|---|
| `frontend/src/components/tasks/kanbanScrollMemory.ts` | NEW — module-scoped Map + helpers |
| `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx` | Add outer-strip ref, `onScroll`, `useLayoutEffect` for horizontal restore. Wire props for column refs. |
| `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx` (`DroppableColumn` inside same file) | Combine new ref with existing dnd-kit `setNodeRef`. Add `onScroll`. Add `useLayoutEffect` for vertical restore (or lift the effect into the parent — see Alternatives). |

## Alternatives Considered

- **Single effect in `SpecTaskKanbanBoard`, refs collected via callback ref
  registry**: cleaner separation (column doesn't know about restoration), but
  requires the parent to know each column's DOM element by id. Implementable
  via `useRef<Map<string, HTMLElement>>()` and a callback ref factory passed
  to `DroppableColumn`. This is the **preferred** approach because it keeps
  all timing logic in one place and avoids `DroppableColumn` (which has 13+
  props already) growing more responsibility. Adopt this for D3.

- **`window.history.scrollRestoration = 'manual'` + native history API**:
  doesn't help because the unmounted board's scroll containers are *inside*
  the page, not the window scroll. Rejected.

- **React Router scroll restoration**: this project uses `react-router5` (per
  `helix/CLAUDE.md`), which does not ship modern scroll-restoration utilities.
  Rejected.

## Gotchas Identified

- The `DroppableColumn`'s scrollable element uses `setNodeRef` from
  `useDroppable`. Combining with a custom ref must call both — write a tiny
  inline composer, don't drop the dnd-kit ref.
- `useLayoutEffect` runs synchronously after DOM mutations; if you read
  `scrollWidth` from a `display: none` ancestor (e.g. board temporarily hidden
  behind the `Paywall` component, `SpecTasksPage.tsx:1224`), the values are
  zero. Guard by checking `el.scrollWidth > el.clientWidth` before applying
  horizontal scroll, and bail (re-attempt next tick) if it's zero — but only
  re-attempt a bounded number of times to avoid loops.
- `event.isTrusted` is `true` for genuine user scrolls and `false` for
  programmatic ones — use this to distinguish our restoration write from a
  user scroll when deciding whether to set `userHasScrolledRef`.
- WIP-limit refresh, search filtering, label filtering all change column
  contents without unmounting the board. None of these should trigger
  restoration — `hasRestoredRef` guards this naturally.
