# Design: Mark Task as Done Without a Pull Request

## Overview

Add a "Mark as Done" option to the task card's three-dot (ellipsis) menu, allowing users to close tasks that don't need a pull request. This is a frontend-only change — the backend already supports direct status updates via `PUT /api/v1/spec-tasks/{taskId}`.

## Architecture

### What Already Exists

- **Backend**: `PUT /api/v1/spec-tasks/{taskId}` accepts `{"status": "done"}` and updates the task. Located in `api/pkg/server/spec_driven_task_handlers.go`.
- **Frontend service**: `useUpdateSpecTask()` in `frontend/src/services/specTaskService.ts` can update any task field including status.
- **Reopen flow**: The existing "Reopen" button in `SpecTaskActionButtons.tsx` already handles reopening done tasks, so no changes needed there.

### Changes Required

**1. TaskCard.tsx** — Add "Mark as Done" menu item

Add a new entry to the three-dot menu in `frontend/src/components/tasks/TaskCard.tsx` (around line 796-946 where the other menu items are defined). Show it for all tasks where `status !== "done"` and `!isArchived`.

The menu item should use a `CheckCircle` icon (from lucide-react, consistent with the done/completed visual language) and trigger a confirmation dialog.

**2. Confirmation Dialog** — Reuse existing pattern

Use the same confirmation dialog pattern as "Archive" (`ArchiveConfirmDialog.tsx`). The dialog should:
- Warn that this will mark the task as complete
- Note that running agents will be stopped (if applicable)
- Allow Shift+click to skip the dialog (same UX as archive)

**3. specTaskWorkflowService.ts** — Add `useMarkTaskDone()` hook

Add a new mutation hook that:
1. Stops any running agent (if the task has one — call the stop-agent endpoint first)
2. Sets status to `done` via `useUpdateSpecTask()`
3. Invalidates relevant query caches

This follows the same pattern as `useMoveToBacklog()` which also stops agents before transitioning.

### Key Decision

**Why the three-dot menu instead of a dedicated button?**

The `SpecTaskActionButtons` component renders primary workflow actions (Start Planning, Accept, etc.). "Mark as Done" is not part of the normal workflow — it's an override/escape hatch. Placing it in the three-dot menu keeps it accessible without cluttering the primary action bar, and is consistent with other override actions like "Move to Backlog" and "Archive".

### Codebase Patterns

- This project uses React Query (TanStack Query) for data fetching/mutations
- Workflow service hooks follow the pattern: call API → invalidate queries → show toast
- The three-dot menu is built with a custom dropdown component using lucide-react icons
- The frontend API client is auto-generated (`frontend/src/api/api.ts`) from the backend OpenAPI spec
