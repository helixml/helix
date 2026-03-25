# Design: Tooltip for Truncated Task Titles

## Approach

Use MUI `<Tooltip>` (the standard in this codebase) in three files. All changes are small and self-contained. Multi-line support via `whiteSpace: "pre-wrap"` on the tooltip text node.

## Key Files

- `frontend/src/components/tasks/TaskCard.tsx` — Kanban card title
- `frontend/src/components/tasks/TabsView.tsx` — PanelTab split-screen tab headings
- `frontend/src/components/system/GlobalNotifications.tsx` — Notifications panel rows

---

## Change 1: Kanban card (TaskCard.tsx)

### Problem
`task.name` is rendered in a `<Typography>` (~line 784) with `wordBreak: "break-word"` and no tooltip. The `SpecTaskWithExtras` interface does not include `original_prompt` or `description`.

### Solution
1. Add `original_prompt?: string` and `description?: string` to the `SpecTaskWithExtras` interface (~line 153). These fields already exist on `BoardTask` (which extends `SpecTaskWithExtras` in `SpecTaskKanbanBoard.tsx`) so no data plumbing is needed — they flow through at runtime.

2. Wrap the task name `<Typography>` with:
```tsx
<Tooltip
  title={
    <span style={{ whiteSpace: "pre-wrap" }}>
      {task.description || task.original_prompt || task.name}
    </span>
  }
  placement="top"
  enterDelay={500}
  arrow
>
  <Typography ...>{task.name}</Typography>
</Tooltip>
```

---

## Change 2: Tab title (TabsView.tsx — PanelTab)

### Problem
The `PanelTab` component already wraps each tab in a MUI `Tooltip` (~line 518). Its `tooltipContent` shows "Topic Evolution" (planning session title history) when a session exists, or `displayTask?.name || displayTask?.description` when no session. The full `original_prompt` is never shown.

### Solution
Update the `tooltipContent` memo so the no-session / no-title-history branch uses `description || original_prompt || name`, wrapped with `whiteSpace: "pre-wrap"`:
```tsx
// no-session branch:
<span style={{ whiteSpace: "pre-wrap" }}>
  {displayTask?.description || displayTask?.original_prompt || displayTask?.name || "Task details"}
</span>
```

The existing tooltip styling (maxWidth 350, styled paper background) handles multi-line content fine. The "Topic Evolution" branch for tasks with planning sessions is unchanged.

`displayTask` comes from `useTask` which already returns `description` and `original_prompt` from the API.

---

## Change 3: Notifications panel (GlobalNotifications.tsx)

### Problem
Each notification row (~line 220) has two text lines truncated with `textOverflow: "ellipsis"` and `whiteSpace: "nowrap"`, with no tooltip:
- Bold title line: `event.title`
- Caption subtitle: `event.spec_task_name || event.spec_task_id`

A `<Tooltip title="Dismiss">` exists on the dismiss button, but nothing on the row text.

### Solution
Wrap the `<Box sx={{ minWidth: 0, flex: 1 }}>` that contains both text lines in a `<Tooltip>` showing the full title and task name:
```tsx
<Tooltip
  title={
    <span style={{ whiteSpace: "pre-wrap" }}>
      {event.title}
      {event.spec_task_name ? `\n${event.spec_task_name}` : ""}
    </span>
  }
  placement="left"
  enterDelay={500}
  arrow
>
  <Box sx={{ minWidth: 0, flex: 1 }}>
    {/* existing title + subtitle Typography */}
  </Box>
</Tooltip>
```

`placement="left"` keeps the tooltip from overlapping the dismiss button on the right.

---

## What NOT to change

- `SpecTaskDetailContent.tsx` — Description section already shows full prompt untruncated with `whiteSpace: "pre-wrap"`.
- Tab "Topic Evolution" tooltip content — keep for tasks with planning sessions.
- Tab context menu items in TabsView (lines ~1345-1386) — these are interactive menus, not static text; the user can scroll or the menu expands naturally.

## Codebase Patterns Noted

- All tooltips use `import { Tooltip } from "@mui/material"` — no custom tooltip component.
- Multi-line tooltip content: wrap text in `<span style={{ whiteSpace: "pre-wrap" }}>` inside the `title` prop.
- Standard enter delay: `enterDelay={500}` (matches PanelTab's existing tooltip).
- `SpecTaskWithExtras` is defined in `TaskCard.tsx` and imported elsewhere; adding optional fields is safe.
- `GlobalNotifications.tsx` uses dark background (`rgba(255,255,255,...)` colors) — default MUI tooltip styling is fine since it uses its own background.
