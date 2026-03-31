# Design: Show Task Priority on Kanban Card

## Context

The `TaskCard` component (`frontend/src/components/tasks/TaskCard.tsx`) already receives the full task object, which includes `priority?: string`. Priority values are `low | medium | high | critical` (from `TypesSpecTaskPriority` enum in `api.ts`).

Priority is currently used to **sort** tasks within columns (critical floats to top) but is not displayed on the card itself. It is shown in:
- The backlog table view (`BacklogTableView.tsx`) — as a coloured `<Chip>`
- The task detail panel — as a `<Chip>` with `getPriorityColor()`

## Approach

Add a small priority label to the **status row** of the card (the row at line ~983 that shows the phase dot + phase name + running duration + assignee avatar). Place it between the phase text and the assignee avatar, or inline after the phase name.

Use a **small dot + muted text label** pattern — no chip border, no exclamation mark. Example: `● high` in the appropriate muted colour. For `medium` and `low` use even more subdued styling (opacity 0.6) so only `high` and `critical` draw any attention. Skip rendering entirely when priority is `medium` (the default/expected case) to reduce noise — only show when it's noteworthy (`low`, `high`, `critical`), or show all but style `medium` and `low` very dimly.

**Decision: show all priorities, but calibrate visual weight by level:**
- `critical` — accent color `#ef4444` (error red), opacity 1.0
- `high` — `#f59e0b` (warning amber), opacity 1.0
- `medium` — `text.secondary`, opacity 0.45 (nearly invisible)
- `low` — `text.secondary`, opacity 0.45

This way `critical` and `high` are glanceable, while `medium` and `low` are discoverable but not distracting.

## Implementation

**File:** `frontend/src/components/tasks/TaskCard.tsx`

In the status row `<Box>` (around line 983), add a priority indicator element after the phase text:

```tsx
{task.priority && task.priority !== "medium" && (
  <Tooltip title={`Priority: ${task.priority}`}>
    <Typography
      variant="caption"
      sx={{
        fontSize: "0.65rem",
        color:
          task.priority === "critical"
            ? "#ef4444"
            : task.priority === "high"
              ? "#f59e0b"
              : "text.secondary",
        opacity: task.priority === "low" ? 0.5 : 1,
        fontWeight: 500,
        textTransform: "capitalize",
        lineHeight: 1,
      }}
    >
      {task.priority}
    </Typography>
  </Tooltip>
)}
```

Preceded by a `•` separator matching the existing `runningDuration` separator pattern.

**No new dependencies required.** No new helper functions needed — the color logic is simple enough inline given only 4 cases.

## Pattern Notes

- The existing colour map (`critical=error red`, `high=amber`, `medium=info blue`, `low=success green`) is defined in `BacklogTableView.tsx`. This design intentionally diverges for the card — using `text.secondary` for low/medium keeps cards calm. The backlog table can remain louder since it's a data table context.
- The status row already uses `gap: 1.5` between elements and `•` as a separator for the running duration — use the same separator pattern.
- `medium` is the most common default, so hiding it avoids 80% of cards showing a label that adds no information.
