# Requirements: Backlog Table View with Editable Fields

## User Stories

### 1. Expand Backlog to Table View
**As a** project manager  
**I want to** click on the backlog column header to expand it into a full table view  
**So that** I can see all backlog tasks at once with full details including the entire prompt

### 2. Edit All Fields Inline
**As a** user managing tasks  
**I want to** edit task name, priority, type, and prompt directly in the table  
**So that** I can quickly update multiple tasks without opening detail views

### 3. View and Edit Full Prompt
**As a** user refining task requirements  
**I want to** see the full prompt text and edit it easily in the table  
**So that** I can refine task descriptions without extra clicks

### 4. Auto-Sort by Priority
**As a** user viewing the backlog  
**I want** tasks to automatically sort by priority (critical → high → medium → low)  
**So that** the most important tasks are always visible at the top

### 5. Filter and Search Tasks
**As a** user with many backlog items  
**I want to** filter and search tasks in the table view  
**So that** I can quickly find specific tasks

## Acceptance Criteria

### Table View Expansion
- [ ] Clicking backlog column header expands to full-width table view
- [ ] Table shows columns: Name, Priority, Type, Prompt (full text), Created
- [ ] Click header again or "X" button to collapse back to kanban column
- [ ] Table view maintains scroll position when collapsing/expanding

### Inline Editing (All Cells)
- [ ] **Name**: Click to edit inline (text input)
- [ ] **Priority**: Click to show dropdown (Critical, High, Medium, Low)
- [ ] **Type**: Click to show dropdown (feature, bug, task, epic)
- [ ] **Prompt**: Click to expand into multi-line textarea for full viewing/editing
- [ ] All changes trigger immediate API update via `v1SpecTasksUpdate`
- [ ] Show loading indicator during save
- [ ] Show error toast if update fails

### Prompt Column
- [ ] Show truncated preview (~100 chars) in table row
- [ ] Click to expand row and show full prompt in textarea
- [ ] Textarea is editable with save/cancel buttons

### Auto-Sort Behavior
- [ ] Tasks sort by priority: critical (top) → high → medium → low (bottom)
- [ ] After priority change, table re-renders with new sort order (no animation needed)
- [ ] Secondary sort by created date (newest first) within same priority

### Filter Bar
- [ ] Text search filters by task name and prompt content
- [ ] Priority filter dropdown (multi-select)
- [ ] Clear filters button when any filter is active
- [ ] Filter state persists during session (not across page refresh)