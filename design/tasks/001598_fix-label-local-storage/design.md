# Design: Fix Label Local Storage

## Root Cause

Two separate `labelFilter` states exist in the project task view:

1. **`SpecTaskKanbanBoard.tsx` line 640** — drives the Autocomplete label filter always visible in the kanban toolbar. Initialised as `useState<string[]>([])` with **no localStorage persistence**. This is the filter the user normally interacts with.

2. **`BacklogTableView.tsx` line 80** — drives filtering inside the expanded backlog table panel. Already has localStorage persistence added by commit 8b08006. This component is only mounted when the user explicitly expands the backlog panel, so the localStorage key is rarely written.

Because the user is using the kanban toolbar filter (case 1), the `helix-label-filter-${projectId}` key is never written, which is why it doesn't appear in Chrome DevTools.

## Pattern Found in Codebase

`BacklogTableView.tsx` already demonstrates the correct pattern:

```typescript
const labelStorageKey = projectId ? `helix-label-filter-${projectId}` : null;

const [labelFilter, setLabelFilter] = useState<string[]>(() => {
  if (!labelStorageKey) return [];
  try {
    const stored = localStorage.getItem(labelStorageKey);
    return stored ? JSON.parse(stored) : [];
  } catch {
    return [];
  }
});

useEffect(() => {
  if (!labelStorageKey) return;
  if (labelFilter.length === 0) {
    localStorage.removeItem(labelStorageKey);
  } else {
    localStorage.setItem(labelStorageKey, JSON.stringify(labelFilter));
  }
}, [labelFilter, labelStorageKey]);
```

## Fix

Apply the same pattern to the `labelFilter` state in `SpecTaskKanbanBoard.tsx`:

- Use the same storage key: `helix-label-filter-${projectId}` — this is intentional: both the kanban toolbar and the backlog table view filter the same project's tasks, so sharing the key means a consistent filter is restored regardless of which view the user last used.
- Replace the simple `useState<string[]>([])` at line 640 with the lazy-initializer that reads from localStorage.
- Add the `useEffect` to sync changes back to localStorage.
- Add `useEffect` import if not already present (it is already imported in that file via `React.useEffect` or directly).

## File to Change

- `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx` — around line 640, the `labelFilter` state declaration.

No other files need changing. `BacklogTableView.tsx` already works correctly and uses the same key, so both views will share the persisted state.

## Codebase Notes

- `helix-4` is a symlink to `helix` — use `/home/retro/work/helix/` for edits.
- Frontend uses Vite HMR in dev mode; after editing, refresh the browser.
- If `FRONTEND_URL=/www` in `.env`, run `cd frontend && yarn build` after changes.
- `projectId` in `SpecTaskKanbanBoard` comes from `SpecTasksPage.tsx` via `router.params.id` — it's a string when on a project page, so the guard `projectId ?` is still needed for safety.
