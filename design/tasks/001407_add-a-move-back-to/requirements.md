# Requirements: Move Back to Backlog Button

## User Stories

**As a user**, I want to move an active task back to the backlog when I decide I don't want to work on it right now, so I can defer it without losing the task.

## Acceptance Criteria

1. A "Move to Backlog" button/menu option is available for tasks in planning, review, and implementation phases
2. Clicking the button stops any running agent and resets the task status to `backlog`
3. The action is available in both the TaskCard (Kanban board) and SpecTaskDetailContent (detail view)
4. User receives feedback (snackbar) confirming the task was moved
5. The task appears back in the Backlog column after the action completes

## Out of Scope

- Moving completed/merged tasks back to backlog
- Preserving generated specs when moving back (they remain but task restarts from scratch if re-started)
- Batch move multiple tasks at once