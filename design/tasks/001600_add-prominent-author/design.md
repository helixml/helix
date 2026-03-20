# Design: Add Prominent Author Field in SpecTask Details

## Context

The `created_by` string field already exists on the `SpecTask` struct (Go: `simple_spec_task.go:150`, TypeScript: `TypesSpecTask.created_by`). It is populated at task creation via the user's email/ID but is never displayed in the UI.

The detail panel is rendered by `SpecTaskDetailContent.tsx`. The timestamps block at lines 1292–1306 is the natural home for an author row — it's in the metadata sidebar, visible at a glance.

## Decision

Add a "Created by:" row to the existing timestamps block in `SpecTaskDetailContent.tsx`, directly above the "Created:" timestamp. Use the same `Typography variant="caption"` style as the surrounding rows — no new components needed.

Only render the row when `task.created_by` is truthy, to avoid empty state.

```tsx
{/* Timestamps */}
<Box sx={{ mt: 3 }}>
  {task?.created_by && (
    <Typography variant="caption" color="text.secondary" display="block">
      Author: {task.created_by}
    </Typography>
  )}
  <Typography variant="caption" color="text.secondary" display="block">
    Created: {task?.created_at ? new Date(task.created_at).toLocaleString() : "N/A"}
  </Typography>
  <Typography variant="caption" color="text.secondary" display="block">
    Updated: {task?.updated_at ? new Date(task.updated_at).toLocaleString() : "N/A"}
  </Typography>
</Box>
```

## Why This Approach

- **Zero backend changes** — field is already in the API response.
- **Minimal diff** — single insertion in one file.
- **Consistent style** — matches the existing timestamp rows without introducing new design patterns.
- **No generated API client changes needed** — `created_by` is already in the TypeScript `TypesSpecTask` interface at `frontend/src/api/api.ts`.

## Patterns Observed

- Project uses MUI `Typography` with `variant="caption" color="text.secondary"` for metadata rows in the detail panel.
- Conditional rendering of optional fields uses `{field && (<.../>)}` pattern throughout the component.
- CLAUDE.md: always use generated API client; no raw fetch. This change is read-only UI — no API calls needed.
