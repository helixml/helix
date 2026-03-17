# Design: Fix Backlog Task Ordering

## Bug Location

Two components share identical (broken) sort logic:

### `frontend/src/components/tasks/BacklogTableView.tsx` (lines 116–129)
```typescript
result.sort((a, b) => {
  const priorityA = PRIORITY_ORDER[a.priority || "medium"] ?? 2;
  const priorityB = PRIORITY_ORDER[b.priority || "medium"] ?? 2;
  if (priorityA !== priorityB) return priorityA - priorityB;

  // BUG: a.created does not exist — field is a.created_at
  const dateA = new Date(a.created || 0).getTime();
  const dateB = new Date(b.created || 0).getTime();
  return dateB - dateA;
});
```

### `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx` (lines 805–819, backlog column only)
```typescript
.sort((a, b) => {
  const priorityA = PRIORITY_ORDER[a.priority || "medium"] ?? 2;
  const priorityB = PRIORITY_ORDER[b.priority || "medium"] ?? 2;
  if (priorityA !== priorityB) return priorityA - priorityB;

  // BUG: a.created does not exist — field is a.created_at
  return new Date(b.created || 0).getTime() - new Date(a.created || 0).getTime();
})
```

## Fix

Replace `a.created` / `b.created` with `a.created_at` / `b.created_at` in both sort blocks.

The `TypesSpecTask` interface (`frontend/src/api/api.ts` line 4767) defines:
```typescript
created_at?: string;
```
There is no `created` field on `TypesSpecTask`.

## Key Architectural Notes

- **Generated API client**: `TypesSpecTask` is generated from the Go `SpecTask` struct via `./stack update_openapi`. The Go struct has `CreatedAt time.Time \`json:"created_at"\`` — so `created_at` is the correct JSON key.
- **`status_updated_at` vs `created_at`**: The DB sorts by `status_updated_at` for Kanban column ordering, but the frontend backlog should sort by `created_at` (creation time) for predictable ordering — a newly created task should always be newest.
- **PRIORITY_ORDER constant**: Defined locally in both files. This is intentional duplication; do not attempt to deduplicate as part of this fix.
- **Other Kanban columns** (planning, review, implementation) do not sort themselves — they rely on the API's `status_updated_at DESC` ordering. No changes needed for those.

## Codebase Patterns Observed

- React Query used for all API calls; no raw fetch.
- TypeScript types are generated — don't edit `api.ts` manually.
- Task sorting is done entirely on the frontend (backend returns unsorted from frontend's perspective, re-sorted in `useMemo`).
