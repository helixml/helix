# Requirements: Fix Label Local Storage

## User Story

As a user, when I filter tasks by label on a project page, I want my label selection to persist so that when I navigate away and return to the same project, my filters are still applied.

## Background

A previous commit (8b08006) added localStorage persistence for label filters in `BacklogTableView.tsx`. However, the `SpecTaskKanbanBoard` component has its **own** separate `labelFilter` state (line 640) that drives the always-visible label filter in the kanban toolbar. This one was **not** given localStorage persistence, so filters set via the kanban toolbar are lost on navigation.

The user reports that `helix-label-filter-${projectId}` key never appears in Chrome DevTools — confirming the write never happens (because the kanban board's filter doesn't write to localStorage, only the backlog table view does).

## Acceptance Criteria

- [ ] Selecting a label in the kanban board toolbar persists the filter to `localStorage` under key `helix-label-filter-${projectId}`
- [ ] On returning to the same project page, the previously selected label filters are restored
- [ ] When all labels are cleared, the `localStorage` key is removed (same cleanup behaviour as BacklogTableView)
- [ ] If `projectId` is undefined, no localStorage operations occur (safe fallback)
- [ ] `helix_last_task_labels` (task-creation pre-population) is unaffected
