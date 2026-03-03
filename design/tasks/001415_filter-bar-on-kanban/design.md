# Design: Filter Bar Task Number Search

## Summary
Extend the `filterTasks` function in `SpecTaskKanbanBoard.tsx` to also match against `task_number` when the search filter is numeric.

## Current Implementation

The filter function in `SpecTaskKanbanBoard.tsx` (lines 750-762) currently searches:
- `task.name`
- `task.description`
- `task.implementation_plan`

```typescript
const filterTasks = (taskList, filter) => {
  if (!filter.trim()) return taskList;
  const lowerFilter = filter.toLowerCase();
  return taskList.filter(
    (task) =>
      task.name?.toLowerCase().includes(lowerFilter) ||
      task.description?.toLowerCase().includes(lowerFilter) ||
      task.implementation_plan?.toLowerCase().includes(lowerFilter),
  );
};
```

## Proposed Change

Add a check for numeric filters that also matches against `task_number`:

```typescript
const filterTasks = (taskList, filter) => {
  if (!filter.trim()) return taskList;
  const lowerFilter = filter.toLowerCase();
  const numericFilter = /^\d+$/.test(filter.trim()) ? parseInt(filter.trim(), 10) : null;
  
  return taskList.filter(
    (task) =>
      task.name?.toLowerCase().includes(lowerFilter) ||
      task.description?.toLowerCase().includes(lowerFilter) ||
      task.implementation_plan?.toLowerCase().includes(lowerFilter) ||
      (numericFilter !== null && task.task_number === numericFilter),
  );
};
```

## Key Decisions

1. **Exact numeric match** - When the filter is purely numeric, match tasks where `task_number` equals that integer. This handles "1412" matching task #001412.

2. **Additive matching** - The task number check is added with `||`, so existing text matches continue to work. If "1412" appears in a task name, that task still matches.

3. **Zero-padded format handled implicitly** - Searching "001412" matches via the text fields (since task names/descriptions might include the padded format), while "1412" matches via the numeric comparison.

## Files to Modify

| File | Change |
|------|--------|
| `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx` | Update `filterTasks` function (~line 750) |

## Testing

1. Create tasks with known task numbers
2. Search by number without leading zeros → should find matching task
3. Search by full padded format → should find matching task
4. Search by text that contains digits → should match via text search
5. Verify existing text search still works

## Implementation Notes

**Change made:** Updated `filterTasks` function at line ~753 in `SpecTaskKanbanBoard.tsx`

```typescript
// Added these lines:
const trimmedFilter = filter.trim();
const numericFilter = /^\d+$/.test(trimmedFilter)
  ? parseInt(trimmedFilter, 10)
  : null;

// Added to filter condition:
(numericFilter !== null && task.task_number === numericFilter)
```

**Build verification:** `cd frontend && yarn build` - completed successfully with no errors.

**Pattern discovered:** The codebase uses `task.task_number` (number type) for storage and displays it with `String(task.task_number).padStart(6, "0")` for UI. The filter needed to compare against the raw integer, not the padded string.