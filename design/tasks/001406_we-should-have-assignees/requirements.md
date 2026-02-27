# Requirements: Task Assignees on Kanban Board

## Overview
Add the ability to assign team members to tasks on the Kanban board to enable coordination between teammates.

## User Stories

### US1: Assign a team member to a task
As a project member, I want to assign a teammate to a task so that everyone knows who is responsible for it.

**Acceptance Criteria:**
- [ ] Tasks display an "assignee" field (avatar or placeholder)
- [ ] Clicking the assignee area opens a dropdown of organization members
- [ ] Selecting a member assigns them to the task
- [ ] Assignment persists and syncs across clients

### US2: View task assignments
As a team member, I want to see who is assigned to each task at a glance on the Kanban board.

**Acceptance Criteria:**
- [ ] Assigned tasks show the assignee's avatar in the TaskCard
- [ ] Unassigned tasks show a placeholder icon
- [ ] Hovering over avatar shows assignee name

### US3: Remove/change assignment
As a project member, I want to unassign or reassign a task.

**Acceptance Criteria:**
- [ ] Assignee dropdown includes "Unassigned" option
- [ ] Selecting a different member replaces the current assignee
- [ ] Changes reflect immediately in UI

### US4: Filter by assignee (stretch goal)
As a team member, I want to filter the Kanban board to show only tasks assigned to a specific person.

**Acceptance Criteria:**
- [ ] Filter dropdown includes list of assignees
- [ ] Selecting an assignee filters visible tasks
- [ ] "All" option shows all tasks

## Out of Scope
- Multiple assignees per task (keep it simple: one assignee)
- Notifications when assigned
- Assignment history/audit trail
- Due date reminders