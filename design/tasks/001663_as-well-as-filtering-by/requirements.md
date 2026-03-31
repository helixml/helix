# Requirements: Assignee Filtering on Kanban Board

## User Story

As a team member viewing the Kanban board, I want to filter tasks by assignee so I can focus on work assigned to specific people (including myself).

## Acceptance Criteria

- [ ] An assignee filter control appears in the Kanban board header, alongside the existing label filter
- [ ] The filter shows org members who have at least one task assigned to them on the board
- [ ] Selecting one or more assignees shows only tasks assigned to those people
- [ ] An "Unassigned" option filters to tasks with no assignee
- [ ] Multiple assignees selected uses OR semantics (tasks assigned to any selected person are shown)
- [ ] The selected assignee filter persists across page refreshes (localStorage, keyed by project ID)
- [ ] Clearing the filter shows all tasks again
- [ ] The filter works correctly on all Kanban columns
