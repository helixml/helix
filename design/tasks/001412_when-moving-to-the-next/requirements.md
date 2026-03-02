# Requirements: Kanban Task Sorting on Status Change

## Problem Statement

When a task is moved to a different column on the Kanban board (status change), it appears at the bottom of the new column instead of at the top. This makes it hard to find the task after moving it.

## User Stories

1. **As a user**, when I drag a task to a new column, I want it to appear at the top of that column so I can easily see where it went.

2. **As a user**, when a task's status is updated programmatically (e.g., by an agent), I want to see it at the top of its new column to notice the change.

## Acceptance Criteria

- [ ] When a task moves to a new column, it appears at the top of that column
- [ ] Tasks within each column are sorted by most-recently-moved-to-that-status first
- [ ] New tasks created directly into a status appear at the top of that column
- [ ] Existing task ordering behavior is preserved for tasks that haven't changed status

## Out of Scope

- Manual drag-and-drop reordering within a column (future feature)
- Customizable sort options per column