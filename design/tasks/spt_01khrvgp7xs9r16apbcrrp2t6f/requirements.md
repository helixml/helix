# Requirements: Archive All Merged Tasks Button

## User Stories

**As a user**, I want a button in the Merged column header so I can archive all merged tasks at once without archiving them individually.

**As a user**, I want a confirmation dialog before bulk archiving so I don't accidentally archive tasks.

**As a user**, I want a fun ta-da! celebration animation when I archive all merged tasks so the action feels rewarding.

## Acceptance Criteria

1. A small archive/celebrate icon button appears in the Merged column header, next to the task count badge.
2. Clicking the button opens a confirmation dialog (not the existing single-task `ArchiveConfirmDialog`) that says something like "Archive all N merged tasks?" with a Cancel and Confirm action.
3. On confirmation, all tasks in the Merged column (phase=`completed` / status=`done`) are archived via `v1SpecTasksArchivePartialUpdate`.
4. After confirmation (or concurrently with it), a full-screen ta-da! animation plays — confetti or sparkles sweeping across the viewport for ~2–3 seconds, then auto-dismissing.
5. The button is only visible when `column.id === "completed"` and `column.tasks.length > 0`.
6. The button is disabled/hidden when the Merged column is empty (nothing to archive).
7. After archiving completes, the task list refreshes (same pattern as `performArchive`).

## Out of Scope

- Archiving tasks in other columns via this button.
- Persisting the animation state or replaying it.
- Backend changes — all archiving uses the existing `v1SpecTasksArchivePartialUpdate` endpoint.
