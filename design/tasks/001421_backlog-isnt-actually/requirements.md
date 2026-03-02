# Requirements: Backlog Priority Sorting in Kanban View

## Problem Statement

The backlog column in the kanban board (`SpecTaskKanbanBoard.tsx`) displays tasks in arbitrary order rather than sorted by priority. Users expect critical/high priority tasks to appear at the top.

## User Stories

1. **As a project manager**, I want backlog tasks sorted by priority (critical → high → medium → low) so I can quickly see the most important work.

2. **As a developer**, I want consistent sorting between the backlog table view and kanban view so tasks appear in the same order regardless of view mode.

## Acceptance Criteria

- [ ] Backlog column tasks are sorted by priority: critical first, then high, medium, low
- [ ] Secondary sort by created date (newest first) when priorities are equal
- [ ] Sorting matches the existing `BacklogTableView.tsx` implementation
- [ ] Other kanban columns remain unaffected (they have their own ordering logic)

## Out of Scope

- User-configurable sort order
- Drag-and-drop reordering within backlog
- Sorting for non-backlog columns