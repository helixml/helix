# Requirements: Don't Download Session Objects on the Kanban View

## Problem

The Kanban (specs) page calls `GET /api/v1/sessions/{id}` every 3 seconds for every task card via `useSandboxState` in `ExternalAgentDesktopViewer.tsx`. Session objects are very large (full interaction history). With many tasks visible, this causes hundreds of MB of unnecessary data transfer just from browsing the board.

The hook only needs 4 small fields from the session config — and the `listTasks` backend handler already batch-fetches those same sessions to populate `session_updated_at`. Those sessions contain the sandbox state fields; they're just not returned.

## User Stories

- As a user viewing the Kanban board, I should not be downloading hundreds of MB of data just by looking at the page.
- As a user with many spec tasks, the Kanban page should remain fast regardless of how many task cards are visible.

## Acceptance Criteria

1. `GET /api/v1/spec-tasks` includes sandbox state fields inline on each task.
2. The Kanban page makes **zero** calls to `GET /api/v1/sessions/{id}`.
3. Sandbox state (running / starting / absent) still displays correctly on all task cards.
4. Status messages (e.g. "Unpacking build cache") still display correctly.
5. State updates when the task list refreshes (consistent with existing refresh cadence).
