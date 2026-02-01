# Requirements: Backlog Table View with Editable Priority

## User Stories

### 1. Expand Backlog to Table View
**As a** project manager  
**I want to** click on the backlog column header to expand it into a full table view  
**So that** I can see all backlog tasks at once with more detail

### 2. Edit Priority Inline
**As a** user managing tasks  
**I want to** edit task priority directly in the table without opening a detail view  
**So that** I can quickly reprioritize multiple tasks

### 3. Auto-Sort by Priority
**As a** user viewing the backlog  
**I want** tasks to automatically sort by priority (critical → high → medium → low)  
**So that** the most important tasks are always visible at the top

### 4. Filter and Search Tasks
**As a** user with many backlog items  
**I want to** filter and search tasks in the table view  
**So that** I can quickly find specific tasks

## Acceptance Criteria

### Table View Expansion
- [ ] Clicking backlog column header expands to full-width table view
- [ ] Table shows: task name, priority, type, description (truncated), created date
- [ ] Click header again or "X" button to collapse back to kanban column
- [ ] Table view maintains scroll position when collapsing/expanding

### Inline Priority Editing
- [ ] Priority column shows dropdown on click (not hover)
- [ ] Options: Critical, High, Medium, Low
- [ ] Change triggers immediate API update via `v1SpecTasksUpdate`
- [ ] Show loading indicator during save
- [ ] Show error toast if update fails

### Auto-Sort Behavior
- [ ] Tasks sort by priority: critical (top) → high → medium → low (bottom)
- [ ] When priority changes, task animates to new position
- [ ] Secondary sort by created date (newest first) within same priority

### Filter Bar
- [ ] Text search filters by task name and description
- [ ] Priority filter dropdown (multi-select)
- [ ] Clear filters button when any filter is active
- [ ] Filter state persists during session (not across page refresh)