# Add tooltips for truncated task titles; fix canonical prompt field handling

## Summary

- Hovering a Kanban card title, split-screen tab heading, or notification now shows the full prompt text in a tooltip with multi-line support.
- `task.name` is now always derived from `task.description` (recalculated on every description update) and can no longer be edited directly, making `description` the single canonical prompt field.
- Fixed a bug where the backlog inline editor was pre-populated from `original_prompt` instead of `description`, causing edits made in the detail panel to be silently discarded.

## Changes

- `api/pkg/services/spec_driven_task_service.go`: export `GenerateTaskNameFromPrompt` (was package-private)
- `api/pkg/server/spec_driven_task_handlers.go`: remove direct `name` update; auto-recalculate `name` from `description` on every description change
- `frontend/src/components/tasks/BacklogTableView.tsx`: initialize inline prompt editor from `task.description` (falling back to `task.original_prompt` for old tasks)
- `frontend/src/components/tasks/TaskCard.tsx`: add `description?: string` to `SpecTaskWithExtras`; wrap title `<Typography>` in MUI `<Tooltip>` showing full `description || name`
- `frontend/src/components/tasks/TabsView.tsx`: show `description || name` (with `whiteSpace: "pre-wrap"`) in tab tooltip when no planning session or title history exists
- `frontend/src/components/system/GlobalNotifications.tsx`: wrap notification text box in `<Tooltip>` showing full `event.title` + `event.spec_task_name`
