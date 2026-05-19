# Restore kanban scroll position on return navigation

## Summary

When users click a task on the Kanban board and then navigate back, both
the horizontal column-strip scroll and each column's vertical scroll
position are now restored to where they left off — instead of resetting
to `(0, 0)` and forcing the user to re-find their place.

State is scoped per-project and lives in memory only (no localStorage),
so it survives in-app navigation but is wiped on full reload — it
preserves "the last place you were within this session", not a permanent
preference.

## Changes

- New `frontend/src/components/tasks/kanbanScrollMemory.ts` — module-scoped
  `Map<projectId, { horizontal, columns }>` with get/save helpers.
- `SpecTaskKanbanBoard.tsx`:
  - Outer column-strip `<Box>` gets a ref + synchronous `onScroll`
    handler that persists `scrollLeft` per `projectId` (desktop only).
  - `DroppableColumn` gains `columnBodyRef` + `onColumnScroll` props.
    The parent passes per-column ref-setters and scroll handlers so
    each column's `scrollTop` is saved under its `column.id`.
  - A `useLayoutEffect` restores both surfaces once data has loaded.
    Bails if the user has already scrolled, runs at most once
    successful pass per mount, and clamps any saved value that
    exceeds the current `scrollWidth` / `scrollHeight` (e.g. tasks
    archived since the previous visit).
  - The restore effect re-runs across renders until all targets are
    satisfied — needed because on a warm react-query cache the local
    `tasks` state is briefly empty (and columns have no scrollable
    height) on the first render after remount.
  - Replaces the existing no-op `setNodeRef` stub inside
    `DroppableColumn` (drag-and-drop was already removed earlier).

## Screenshots

![Kanban after restore on empty board](https://github.com/helixml/helix/raw/helix-specs/design/tasks/spt_01krav8qpvfrj1tj8a6r5jmz9z/screenshots/02-kanban-after-restore.png)

![Kanban restored with backlog scrolled](https://github.com/helixml/helix/raw/helix-specs/design/tasks/spt_01krav8qpvfrj1tj8a6r5jmz9z/screenshots/03-kanban-restored-with-tasks.png)

## Test plan

- [x] Scroll horizontal + vertical on a populated board, click away,
  click back — both positions restored.
- [x] Live-save: scroll, navigate, scroll again before navigating
  back — latest value is restored, not the earlier one.
- [x] Clamp: scroll to bottom of a column, archive most of its tasks,
  navigate away and back — scrollTop lands at the new max without
  errors.
- [x] `npx tsc --noEmit` is clean.
- [ ] CI green.
