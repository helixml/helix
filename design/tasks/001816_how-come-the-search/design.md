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

Extract a shared utility function `matchesAllTokens` that both views can use. Split the search input on whitespace, then require **every** token to appear somewhere in the concatenated searchable text (AND logic across tokens). This is the standard "all words must match" behavior users expect from search boxes.

### Shared utility: `frontend/src/utils/searchUtils.ts`

```typescript
export function matchesAllTokens(query: string, ...fields: (string | undefined | null)[]): boolean {
  const trimmed = query.trim();
  if (!trimmed) return true;
  const tokens = trimmed.toLowerCase().split(/\s+/);
  const searchableText = fields.filter(Boolean).join(" ").toLowerCase();
  return tokens.every((token) => searchableText.includes(token));
}
```

### Usage in `SpecTaskKanbanBoard.tsx`

```typescript
import { matchesAllTokens } from '../../utils/searchUtils';

const filterTasks = (taskList: BoardTask[], filter: string): BoardTask[] => {
  if (!filter.trim()) return taskList;
  const trimmedFilter = filter.trim();
  const numericFilter = /^\d+$/.test(trimmedFilter)
    ? parseInt(trimmedFilter, 10)
    : null;

  return taskList.filter((task) => {
    if (numericFilter !== null && task.task_number === numericFilter) return true;
    return matchesAllTokens(filter, task.name, task.description, task.implementation_plan);
  });
};
```

### Usage in `BacklogTableView.tsx`

```typescript
import { matchesAllTokens } from '../../utils/searchUtils';

// In the filter:
result = result.filter((task) => matchesAllTokens(search, task.original_prompt));
```

Key decisions:
- **AND logic** (all words must match), not OR — AND is more useful for narrowing results. The user's example "github public" clearly expects both words to be present.
- **Variadic `...fields` parameter** — callers pass whichever fields are relevant; the utility concatenates and searches them. Keeps it flexible without over-engineering.
- Numeric task number matching stays in `SpecTaskKanbanBoard.tsx` since it's specific to that view (BacklogTableView doesn't have task numbers).

## Scope

Three files:

1. **`frontend/src/utils/searchUtils.ts`** (new) — shared `matchesAllTokens` utility.
2. **`SpecTaskKanbanBoard.tsx`** — `filterTasks` function (line 826). Import and use `matchesAllTokens`.
3. **`BacklogTableView.tsx`** — inline filter (line 87). Import and use `matchesAllTokens`.

No backend changes needed — this is purely client-side filtering of already-loaded tasks.
