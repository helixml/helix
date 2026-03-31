# Design: Assignee Filtering on Kanban Board

## Overview

Mirror the existing label filter pattern in `SpecTaskKanbanBoard.tsx` to add an assignee filter. No backend changes needed — assignee data is already on each task (`assignee_id`) and org members are already available via hooks.

## Key Files

- `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx` — all changes go here
- `frontend/src/components/tasks/AssigneeSelector.tsx` — reference for member list rendering (avatar + name pattern)

## Implementation Design

### State

```typescript
const assigneeStorageKey = projectId ? `helix-assignee-filter-${projectId}` : null;
const [assigneeFilter, setAssigneeFilter] = useState<string[]>(() => {
  if (!assigneeStorageKey) return [];
  try { return JSON.parse(localStorage.getItem(assigneeStorageKey) || "[]"); }
  catch { return []; }
});
```

### Filter Logic

Applied in the existing `filteredTasks` useMemo, after label filtering, using OR semantics:

```typescript
if (assigneeFilter.length > 0) {
  result = result.filter((task) =>
    assigneeFilter.includes(task.assignee_id || "__unassigned__")
  );
}
```

The sentinel value `"__unassigned__"` represents tasks with no assignee.

### Available Assignees (derived from loaded tasks)

```typescript
const availableAssignees = useMemo(() => {
  const ids = new Set<string>();
  tasks.forEach((t) => ids.add(t.assignee_id || "__unassigned__"));
  return Array.from(ids);
}, [tasks]);
```

### UI

Add an `Autocomplete` in the board header immediately after the label filter. Use the org members list (already fetched) to render avatar + display name for each option. For the `"__unassigned__"` option, render "Unassigned" with a `PersonOff` or `AccountCircle` icon.

Persist selection to localStorage on change (same pattern as label filter).

## Decisions

- **OR semantics for assignees**: Natural expectation — "show me Alice's or Bob's tasks", not "show me tasks assigned to both".
- **Derive available assignees from tasks**: Avoids a new API call; only shows assignees relevant to the current board.
- **No backend changes**: `assignee_id` is already returned on every task; org members already fetched for the board.
- **Sentinel `"__unassigned__"`**: Avoids null/undefined in the filter array, keeps state as `string[]` consistent with the label filter pattern.
