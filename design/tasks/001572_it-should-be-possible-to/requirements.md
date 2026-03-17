# Requirements: Change Task Priority Order

## Context

Tasks have a `priority` field (`low`, `medium`, `high`, `critical`) that is stored but **not currently used for Kanban ordering**. The Kanban board sorts tasks by `status_updated_at DESC` (most-recently-moved tasks appear first). Changing priority is buried behind an "edit mode" toggle in the details panel.

Users want priority to feel like a meaningful ordering mechanism — changing it should visually reorder tasks within their column.

## User Stories

**US1:** As a user, I can change a task's priority from the "..." context menu on the task card, without opening the details panel.

**US2:** As a user, I can change a task's priority by clicking directly on the priority chip in the details panel (no full edit mode required).

**US3:** As a user, when I change a task's priority, its position in the Kanban column updates to reflect the new priority (higher priority = closer to the top).

## Acceptance Criteria

- [ ] The "..." menu on TaskCard includes a "Set Priority" submenu or group of options (Critical, High, Medium, Low), with a checkmark on the current value.
- [ ] Clicking a priority option in the "..." menu calls the update API and refreshes the board.
- [ ] The priority chip in the details panel is directly clickable and opens a dropdown/menu to change priority — no need to enter full edit mode first.
- [ ] The Kanban board orders tasks within each column by priority descending (critical → high → medium → low), with ties broken by `status_updated_at DESC`.
- [ ] Changing priority causes immediate visual reordering in the Kanban column.
