# Requirements: Put Current User at Top of Assignees List

## User Story

As a team member using the spectask kanban board, when I open the assignee selector on a task card, I want to see myself at the top of the list so I can quickly assign the task to myself without searching.

## Acceptance Criteria

- [ ] In the `AssigneeSelector` dropdown, the currently logged-in user appears as the first entry in the member list (below "Unassigned")
- [ ] The current user entry is visually identical to other member entries (same avatar, name, email format)
- [ ] When a search query is active, the current user is only shown if they match the search — ordering still applies
- [ ] If the current user is not a member of the organization, no change in behavior (degrade gracefully)
- [ ] All other members remain sorted alphabetically after the current user
