# Design: Filter by Task Number in Split Screen Plus Menu

## Root Cause

`TabsView.tsx` contains a `filteredTasks` memo (lines 786–794) that only matches by task title:

```typescript
const filteredTasks = useMemo(() => {
  if (!taskSearchQuery.trim()) return unopenedTasks;
  const query = taskSearchQuery.toLowerCase();
  return unopenedTasks.filter((task) => {
    const title = task.user_short_title || task.short_title || task.name || "";
    return title.toLowerCase().includes(query);
  });
}, [unopenedTasks, taskSearchQuery]);
```

`SpecTaskKanbanBoard.tsx` already handles numeric queries correctly (lines 756–767) by checking if the trimmed input is purely digits and comparing to `task.task_number`. `TabsView.tsx` simply doesn't have this logic.

## Fix

Add the same numeric-first matching pattern to `TabsView.tsx`'s `filteredTasks` memo:

```typescript
const filteredTasks = useMemo(() => {
  if (!taskSearchQuery.trim()) return unopenedTasks;
  const trimmed = taskSearchQuery.trim();
  const numericFilter = /^\d+$/.test(trimmed) ? parseInt(trimmed, 10) : null;
  const query = trimmed.toLowerCase();
  return unopenedTasks.filter((task) => {
    if (numericFilter !== null) return task.task_number === numericFilter;
    const title = task.user_short_title || task.short_title || task.name || "";
    return title.toLowerCase().includes(query);
  });
}, [unopenedTasks, taskSearchQuery]);
```

## Decision

When input is purely numeric, only match by task number (not title). This is consistent with `SpecTaskKanbanBoard.tsx` behavior and avoids false positives from titles that happen to contain digits.

## Pattern Note

`SpecTaskKanbanBoard.tsx` is the reference implementation for this pattern. Any future search inputs that need task-number support should follow the same `/^\d+$/` + `parseInt` + `task.task_number` check.
