# Requirements: Fix Backlog Task Ordering (My Tasks Always Below Others')

## Problem

Users report that in the backlog, other users' tasks consistently appear above their own, even when they've just created a brand new task. A newly created task should appear at the top of its priority group, but instead it sinks below other tasks.

## Root Cause

The backlog sort functions in `BacklogTableView.tsx` and `SpecTaskKanbanBoard.tsx` use `a.created` as the secondary sort key. However, the `TypesSpecTask` TypeScript interface has no `created` field — the correct field is `created_at`. Since `a.created` is always `undefined`, the expression `new Date(a.created || 0).getTime()` evaluates to `0` for every task. The secondary sort is effectively a no-op (all tasks are equal), so the API's original order is preserved.

The API returns tasks ordered by `status_updated_at DESC NULLS LAST, created_at DESC`. Because agents processing other users' tasks update `status_updated_at` frequently (every status change), those tasks float to the top of the same-priority group. The current user's freshly created task has an older `status_updated_at` relative to recently-active tasks, so it sinks to the bottom.

## User Stories

- **As a user**, I want my newly created backlog task to appear at the top of its priority group, so that I can find it immediately after creating it.
- **As a user**, I want the backlog to sort tasks within each priority level by creation date (newest first), so the ordering is predictable and not affected by background agent activity.

## Acceptance Criteria

1. When a user creates a new task (default priority: medium), it appears at the top of the medium-priority section in both the Backlog table view and the Kanban board backlog column.
2. Within the same priority level, tasks are ordered by `created_at` descending (newest first), not by `status_updated_at`.
3. The primary sort (priority: critical → high → medium → low) is unchanged.
4. Other columns on the Kanban board (planning, review, implementation) are unaffected — they may continue using `status_updated_at` ordering if desired.
