# Design: Tooltip for Truncated Task Titles + Canonical Prompt Field

## Decision: `description` is the canonical prompt field

The agent already uses `description` as primary (falling back to `original_prompt` only when empty). All display and editing code should do the same. `original_prompt` remains in the DB as an immutable audit trail but is not a display or edit source.

All tooltip content and fallback chains use `task.description || task.name` (the `|| task.name` covers the theoretical edge case of a very old task with no `description`; in practice both are always set at creation).

---

## Change 1: Auto-recalculate `name` from `description` on update (backend)

`name` is generated at creation by `generateTaskNameFromPrompt(prompt)` (truncates to ~N chars, collapses whitespace). It should stay in sync with `description` automatically and never be set independently.

**In `spec_driven_task_handlers.go` `updateSpecTask` handler:**
- Remove the `if updateReq.Name != ""` block that lets `name` be set directly.
- Instead, whenever `description` is updated, recalculate `name`:
```go
if updateReq.Description != "" {
    task.Description = updateReq.Description
    task.Name = generateTaskNameFromPrompt(updateReq.Description)
}
```

`generateTaskNameFromPrompt` is already defined in `spec_driven_task_service.go` — move it to a shared location (or keep it in the service and call it from the handler via a helper).

---

## Change 2: Fix backlog inline editor (BacklogTableView.tsx)

### Problem (bug)
`handlePromptClick` (line 140) initializes the editor with `task.original_prompt`:
```ts
setEditingPrompt(task.original_prompt || "");
```
But `handlePromptSave` saves to `description`. So if the user edits from the detail panel, that edit is lost the next time they open the inline editor from the backlog.

### Fix
```ts
setEditingPrompt(task.description || task.original_prompt || "");
```
Prefer `description`; fall back to `original_prompt` only for old tasks that predate the field split.

---

## Change 3: Kanban card tooltip (TaskCard.tsx)

Wrap the task name `<Typography>` in a MUI `<Tooltip>`:
```tsx
<Tooltip
  title={<span style={{ whiteSpace: "pre-wrap" }}>{task.description || task.name}</span>}
  placement="top"
  enterDelay={500}
  arrow
>
  <Typography ...>{task.name}</Typography>
</Tooltip>
```

`task.description` and `task.original_prompt` are not currently in the `SpecTaskWithExtras` interface in `TaskCard.tsx`. Add `description?: string` to that interface so it flows through from the API response.

---

## Change 4: Tab heading tooltip (TabsView.tsx — PanelTab)

The existing `tooltipContent` memo has a branch for tasks with no planning session / no title history. Update that branch to show `description`:

```tsx
// no-session/no-history branch:
<span style={{ whiteSpace: "pre-wrap" }}>
  {displayTask?.description || displayTask?.name || "Task details"}
</span>
```

The "Topic Evolution" branch (tasks with a planning session) is unchanged.

`displayTask` comes from `useTask`, which already returns `description` from the API.

---

## Change 5: Notifications panel tooltip (GlobalNotifications.tsx)

Wrap the text content `<Box>` (containing both truncated lines) in a single `<Tooltip>`:

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
    {/* existing title + subtitle Typography — unchanged */}
  </Box>
</Tooltip>
```

`placement="left"` keeps the tooltip from overlapping the dismiss button on the right.

---

## What NOT to change

- `original_prompt` backend field — keep as-is (immutable audit trail, agent fallback).
- `SpecTaskDetailContent.tsx` description display — already shows full untruncated text.
- Tab "Topic Evolution" tooltip content — keep for tasks with planning sessions.
- The `description || original_prompt` fallback in `spec_driven_task_service.go` — already correct.

## Codebase Patterns Noted

- All tooltips use `import { Tooltip } from "@mui/material"` — no custom tooltip component.
- Multi-line tooltip text: wrap in `<span style={{ whiteSpace: "pre-wrap" }}>` inside the `title` prop.
- Standard enter delay is `enterDelay={500}`.
- At task creation, `description` and `original_prompt` are set to the same value; they only diverge after the user edits `description`.
- `BacklogTableView.tsx` uses `task.original_prompt` as the initial editor value (bug); fix to use `task.description`.
