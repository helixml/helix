# Design: Backlog Priority Sorting in Kanban View

## Current State

The `SpecTaskKanbanBoard.tsx` component filters tasks into columns but does not sort them:

```typescript
tasks: filteredTasks.filter(
  (t) => (t as any).phase === "backlog" && !t.hasSpecs,
),
```

Meanwhile, `BacklogTableView.tsx` already implements correct priority sorting:

```typescript
const PRIORITY_ORDER: Record<string, number> = {
  critical: 0,
  high: 1,
  medium: 2,
  low: 3,
};

result.sort((a, b) => {
  const priorityA = PRIORITY_ORDER[a.priority || "medium"] ?? 2;
  const priorityB = PRIORITY_ORDER[b.priority || "medium"] ?? 2;
  if (priorityA !== priorityB) return priorityA - priorityB;
  // Secondary sort by created date (newest first)
  const dateA = new Date(a.created || 0).getTime();
  const dateB = new Date(b.created || 0).getTime();
  return dateB - dateA;
});
```

## Solution

Add the same sorting logic to the backlog column in `SpecTaskKanbanBoard.tsx`.

### Approach

1. Extract a reusable `sortByPriority` helper function
2. Apply it to the backlog column's task list after filtering

### Code Location

File: `helix/frontend/src/components/tasks/SpecTaskKanbanBoard.tsx`

Add sorting at ~line 780 where backlog tasks are filtered:

```typescript
// Before
tasks: filteredTasks.filter(
  (t) => (t as any).phase === "backlog" && !t.hasSpecs,
),

// After
tasks: filteredTasks
  .filter((t) => (t as any).phase === "backlog" && !t.hasSpecs)
  .sort((a, b) => {
    const PRIORITY_ORDER: Record<string, number> = { critical: 0, high: 1, medium: 2, low: 3 };
    const priorityA = PRIORITY_ORDER[a.priority || "medium"] ?? 2;
    const priorityB = PRIORITY_ORDER[b.priority || "medium"] ?? 2;
    if (priorityA !== priorityB) return priorityA - priorityB;
    return new Date(b.created || 0).getTime() - new Date(a.created || 0).getTime();
  }),
```

### Alternative: Shared Utility

Could extract to a shared utility in `helix/frontend/src/utils/taskSorting.ts` and use in both `BacklogTableView.tsx` and `SpecTaskKanbanBoard.tsx`. However, the sorting logic is simple enough that inline is acceptable for now.

## Testing

1. Create tasks with different priorities in backlog
2. Verify kanban view shows critical → high → medium → low
3. Verify tasks with same priority are sorted by newest first
4. Compare with table view to confirm consistency