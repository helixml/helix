# Design: Tooltip for Truncated Task Titles

## Approach

Use MUI `<Tooltip>` (already the standard in this codebase) in two places. Both changes are small and self-contained.

## Key Files

- `frontend/src/components/tasks/TaskCard.tsx` — Kanban card title
- `frontend/src/components/tasks/TabsView.tsx` — PanelTab component (split-screen tab headings)

## Change 1: Kanban card (TaskCard.tsx)

### Problem
`task.name` is rendered in a `<Typography>` with `wordBreak: "break-word"`. In narrow columns it wraps but shows no tooltip. `SpecTaskWithExtras` (the interface for card tasks) does not include `original_prompt` or `description`.

### Solution
1. Add `original_prompt?: string` and `description?: string` to the `SpecTaskWithExtras` interface (line ~153). These fields already exist on the `BoardTask` type in `SpecTaskKanbanBoard.tsx` (which extends `SpecTaskWithExtras`) so no data plumbing is needed - they flow through automatically.

2. Wrap the task name `<Typography>` (~line 784) with:
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

`whiteSpace: "pre-wrap"` preserves newlines in multi-line prompts.

## Change 2: Tab title in TabsView (TabsView.tsx)

### Problem
The `PanelTab` component already has a MUI `Tooltip` wrapping each tab (line ~518). Its `tooltipContent` shows "Topic Evolution" (title history from planning sessions) when a session exists, or `displayTask?.name || displayTask?.description` when no session.

The full original prompt is not shown — even in the no-session case, only `name` or `description` is used, not `original_prompt`.

### Solution
Update the `tooltipContent` memo in `PanelTab` to prefer `original_prompt` over just `name` when no session title history is available:

```tsx
// In the "no session" and "no title history" branches, use:
displayTask?.description || displayTask?.original_prompt || displayTask?.name || "Task details"
```

The existing tooltip styling (maxWidth 350, styled paper background) handles multi-line content well. Add `whiteSpace: "pre-wrap"` to the tooltip text node to preserve newlines.

`displayTask` is the `TaskWithDetails` type fetched by `useTask` — it already contains `description` and `original_prompt` from the API.

## What NOT to change

- `SpecTaskDetailContent.tsx` — the Description section already shows the full prompt untruncated with `whiteSpace: "pre-wrap"`.
- The "Topic Evolution" tooltip content for tasks that have an active planning session — keep that behavior as-is.

## Codebase Patterns Noted

- All tooltips use `import { Tooltip } from "@mui/material"` — no custom tooltip component.
- Multi-line tooltip content: wrap text in `<span style={{ whiteSpace: "pre-wrap" }}>` inside the `title` prop.
- `enterDelay={500}` is the standard delay used in PanelTab's existing tooltip.
- `SpecTaskWithExtras` is defined in `TaskCard.tsx` and imported by `SpecTaskKanbanBoard.tsx`. Adding optional fields to the interface is safe — existing callers pass `BoardTask` objects which already carry these fields at runtime.
