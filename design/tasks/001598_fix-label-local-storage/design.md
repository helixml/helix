# Design: Fix Label Local Storage

## Root Cause

Two separate `labelFilter` states exist in the project task view:

1. **`SpecTaskKanbanBoard.tsx` line 640** — drives the Autocomplete label filter always visible in the kanban toolbar. Initialised as `useState<string[]>([])` with **no localStorage persistence**. This is the correct, single place for label filtering.

2. **`BacklogTableView.tsx` line 80** — a duplicate label filter added inside the expanded backlog table panel by commit 8b08006. This is not needed: the backlog table is part of the same kanban board and should simply respect the toolbar filter passed down from the parent.

The original implementation was wrong in two ways: it added persistence to the wrong component (BacklogTableView instead of SpecTaskKanbanBoard), and it introduced a redundant, separate label filter inside the backlog panel.

## Fix

**Two changes:**

1. **`SpecTaskKanbanBoard.tsx`** — add localStorage persistence to the existing `labelFilter` state (line ~640):
   - Compute `labelStorageKey = projectId ? \`helix-label-filter-${projectId}\` : null` before the state declaration.
   - Replace `useState<string[]>([])` with a lazy initializer that reads from localStorage.
   - Add a `useEffect` that writes/removes the key whenever `labelFilter` changes.

2. **`BacklogTableView.tsx`** — remove the duplicate label filter:
   - Remove the `labelStorageKey` const, `labelFilter` state, and its `useEffect`.
   - Remove `labelFilter` and `onLabelFilterChange` props from `BacklogFilterBar` usage (or remove `BacklogFilterBar` entirely if label filtering is its only purpose).
   - The backlog table view receives its tasks already filtered by the parent kanban board (the `tasks` prop is the backlog column's tasks after the board-level filter is applied), so no separate filter state is needed inside it.

The storage key stays `helix-label-filter-${projectId}`.

## Files to Change

- `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx` — add persistence to the existing `labelFilter` state
- `frontend/src/components/tasks/BacklogTableView.tsx` — remove the duplicate label filter state, `useEffect`, and related props/UI

## Codebase Notes

- `helix-4` is a symlink to `helix` — use `/home/retro/work/helix/` for edits.
- Frontend uses Vite HMR in dev mode; after editing, refresh the browser.
- If `FRONTEND_URL=/www` in `.env`, run `cd frontend && yarn build` after changes.
- `projectId` in `SpecTaskKanbanBoard` comes from `SpecTasksPage.tsx` via `router.params.id` — it's a string when on a project page, so the `projectId ?` guard is still needed for safety.
- Check whether `BacklogFilterBar` is used anywhere else before removing label filter props from it.
