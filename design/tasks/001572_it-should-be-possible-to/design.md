# Design: Change Task Priority Order

## Architecture

### Backend: Sort by Priority

**File:** `api/pkg/store/store_spec_tasks.go` (line ~362)

Change the `ListSpecTasks` ORDER BY from:
```sql
ORDER BY status_updated_at DESC NULLS LAST, created_at DESC
```
to:
```sql
ORDER BY priority_order DESC, status_updated_at DESC NULLS LAST, created_at DESC
```

Use a CASE expression or a computed column to convert priority string to integer:
```sql
CASE priority
  WHEN 'critical' THEN 4
  WHEN 'high'     THEN 3
  WHEN 'medium'   THEN 2
  WHEN 'low'      THEN 1
  ELSE 2
END AS priority_order
```

No schema migration needed — `priority` column already exists.

### Frontend: "..." Menu Priority Change

**File:** `frontend/src/components/tasks/TaskCard.tsx` (menu section ~line 757–907)

Add a nested submenu or a group of `MenuItem` rows for priority selection. Pattern already established by existing menu items. Example:

```tsx
{(['critical', 'high', 'medium', 'low'] as SpecTaskPriority[]).map(p => (
  <MenuItem
    key={p}
    onClick={() => { handlePriorityChange(p); setMenuAnchorEl(null); }}
  >
    {task.priority === p && <CheckIcon fontSize="small" sx={{ mr: 1 }} />}
    {p.charAt(0).toUpperCase() + p.slice(1)}
  </MenuItem>
))}
```

`handlePriorityChange` calls `updateSpecTask({ taskId, updates: { priority: p } })`.

Add a divider before/after to visually separate from status actions.

### Frontend: Inline Priority Click in Details Panel

**File:** `frontend/src/components/tasks/SpecTaskDetailContent.tsx` (priority section ~line 1014–1055)

Currently, the priority chip is only editable inside `isEditMode`. Make the priority chip always clickable — it opens a `Menu` anchored to the chip. This is independent of the general edit mode toggle.

```tsx
// Always render chip as clickable
<Chip
  label={task.priority}
  onClick={(e) => setPriorityMenuAnchor(e.currentTarget)}
  sx={{ cursor: 'pointer', ... }}
/>
<Menu anchorEl={priorityMenuAnchor} open={Boolean(priorityMenuAnchor)} ...>
  {priorities.map(p => <MenuItem onClick={() => savePriority(p)}>{p}</MenuItem>)}
</Menu>
```

## Key Decisions

- **Sort priority client-side or server-side?** Server-side in `ListSpecTasks` — consistent with existing pattern, no extra complexity on frontend.
- **Submenu vs flat menu items?** Flat items with a divider are simpler and match existing menu style. A "Set Priority →" submenu adds a second click; avoid unless menu becomes too long.
- **Inline edit vs modal?** Inline chip-anchored dropdown — matches UX of clicking a status chip in other tools (Linear, Jira). Keeps the panel clean.
- **Priority ordering tie-break:** `status_updated_at DESC` is preserved as secondary sort so recently updated same-priority tasks still bubble up.

## Codebase Notes

- `SpecTaskPriority` type: `"low" | "medium" | "high" | "critical"` — defined in `api/pkg/types/simple_spec_task.go`.
- The `useUpdateSpecTask` mutation in `specTaskService.ts` already handles priority updates; no new API needed.
- The "..." menu uses MUI `Menu` + `MenuItem` components. The existing items are good patterns to follow.
- `dnd-kit` imports exist in the Kanban board but are disabled (comment: "Removed to prevent infinite loops"). Do not re-enable for this task.
