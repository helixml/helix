# Design: Multi-Word Search in Kanban Board

## Root Cause

In `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx` lines 826-846, the `filterTasks` function does:

```typescript
const lowerFilter = filter.toLowerCase();
// ...
task.name?.toLowerCase().includes(lowerFilter)
```

This is a single-substring match. "github public" is never found as a contiguous substring in text where the words are separated by other content.

## Fix

Split the search input on whitespace, then require **every** token to appear somewhere in at least one of the searchable fields (AND logic across tokens, OR logic across fields per token). This is the standard "all words must match" behavior users expect from search boxes.

```typescript
const filterTasks = (taskList: BoardTask[], filter: string): BoardTask[] => {
  if (!filter.trim()) return taskList;
  const trimmedFilter = filter.trim();
  const numericFilter = /^\d+$/.test(trimmedFilter)
    ? parseInt(trimmedFilter, 10)
    : null;

  const tokens = trimmedFilter.toLowerCase().split(/\s+/);

  return taskList.filter((task) => {
    if (numericFilter !== null && task.task_number === numericFilter) return true;

    const searchableText = [
      task.name,
      task.description,
      task.implementation_plan,
    ]
      .filter(Boolean)
      .join(" ")
      .toLowerCase();

    return tokens.every((token) => searchableText.includes(token));
  });
};
```

Key decisions:
- **AND logic** (all words must match), not OR — AND is more useful for narrowing results. The user's example "github public" clearly expects both words to be present.
- **Concatenate fields** before checking — a token can appear in any field. This is simpler and equivalent to checking each field separately with OR.
- Numeric task number matching is preserved as an early-return.

## Scope

Two files need the same change:

1. **`SpecTaskKanbanBoard.tsx`** — `filterTasks` function (line 826). Searches `name`, `description`, `implementation_plan`.
2. **`BacklogTableView.tsx`** — inline filter (line 87). Searches `original_prompt` only.

No backend changes needed — this is purely client-side filtering of already-loaded tasks.
