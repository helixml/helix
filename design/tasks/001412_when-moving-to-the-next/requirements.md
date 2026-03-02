# Requirements: Kanban Task Sorting on Status Change

## Problem Statement

When a task moves to a different column on the Kanban board (status change), it appears at the bottom of the new column instead of at the top. This makes it hard to find the task after it transitions.

## How Tasks Move Between Columns

Tasks move between columns via:
1. **Button clicks**: "Start Planning", "Review Spec", "Approve Implementation", etc. (see `SpecTaskActionButtons.tsx`)
2. **Agent/Git triggers**: Agent pushes design docs to `helix-specs` branch → task moves from `spec_generation` to `spec_review` (see `git_http_server.go:processDesignDocsForBranch`)
3. **API updates**: Direct status changes via `PUT /api/v1/spec-tasks/{taskId}`

Note: Drag-and-drop exists in `AgentKanbanBoard.tsx` but has a `TODO` comment and doesn't call the API.

## User Stories

1. **As a user**, when I click a button that moves a task to a new column (e.g., "Start Planning"), I want it to appear at the top of that column so I can easily see where it went.

2. **As a user**, when an agent completes work and the task automatically transitions (e.g., design docs pushed), I want to see the task at the top of its new column to notice the change.

## Acceptance Criteria

- [ ] When a task moves to a new column, it appears at the top of that column
- [ ] Tasks within each column are sorted by most-recently-moved-to-that-status first
- [ ] New tasks created directly into a status appear at the top of that column
- [ ] Existing task ordering behavior is preserved for tasks that haven't changed status

## Out of Scope

- Manual drag-and-drop reordering within a column (future feature)
- Customizable sort options per column