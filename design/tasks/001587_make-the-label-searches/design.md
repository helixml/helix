# Design: Persist Label Filter in Local Storage

## Approach

Pass `projectId` as a new prop to `BacklogTableView`, then use it to form a per-project
localStorage key when initializing and syncing the `labelFilter` state.

## Key Files

- `frontend/src/components/tasks/BacklogTableView.tsx` — owns `labelFilter` state; needs changes
- `frontend/src/pages/SpecTasksPage.tsx` — renders `BacklogTableView`; has `projectId`
- `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx` — also renders `BacklogTableView`; may need `projectId` too

## localStorage Key

```
helix-label-filter-${projectId}
```

Value stored as JSON array of strings (e.g. `["bug","ui"]`). No TTL needed.

## State Pattern

The existing pattern used in `SpecTasksPage` for similar toggles:

```typescript
const labelStorageKey = `helix-label-filter-${projectId}`;

const [labelFilter, setLabelFilter] = useState<string[]>(() => {
  try {
    const stored = localStorage.getItem(labelStorageKey);
    return stored ? JSON.parse(stored) : [];
  } catch {
    return [];
  }
});

useEffect(() => {
  if (labelFilter.length === 0) {
    localStorage.removeItem(labelStorageKey);
  } else {
    localStorage.setItem(labelStorageKey, JSON.stringify(labelFilter));
  }
}, [labelFilter, labelStorageKey]);
```

## Prop Change

Add `projectId?: string` to `BacklogTableView` props. When `projectId` is undefined (edge
case), fall back to non-persisted behavior (empty initial state, no sync effect).

## Decision: State in BacklogTableView vs SpecTasksPage

Keep state in `BacklogTableView` (no lift needed). The component already manages
`labelFilter`; adding a `projectId` prop is the minimal change. Lifting state would require
threading props through more layers.

## Codebase Patterns Observed

- No custom `useLocalStorage` hook exists — direct `localStorage.getItem/setItem` calls are standard
- `localStorage.getItem` is called inside `useState` initializer for lazy init
- `useEffect` with the state value as dependency syncs changes
- Keys follow the `helix-<feature>` naming convention
