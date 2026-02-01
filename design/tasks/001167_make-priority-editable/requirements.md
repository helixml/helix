# Requirements: Backlog Table View with Editable Priority

## User Stories

### 1. Expand Backlog to Table View
**As a** project manager  
**I want to** click on the backlog column header to expand it into a full table view  
**So that** I can see all my backlog prompts stacked vertically with their full text

### 2. View Full Prompts
**As a** user reviewing backlog items  
**I want to** see the entire prompt text for each task (multiline, not truncated)  
**So that** I can read and understand all my tasks at a glance

### 3. Edit Prompts Inline
**As a** user refining task requirements  
**I want to** edit the prompt text directly in the table  
**So that** I can refine task descriptions without extra clicks

### 4. Edit Priority and Auto-Sort
**As a** user prioritizing work  
**I want to** change task priority via dropdown and have tasks automatically reorder  
**So that** critical tasks always appear at the top

### 5. Filter and Search Tasks
**As a** user with many backlog items  
**I want to** filter and search tasks in the table view  
**So that** I can quickly find specific tasks

## Acceptance Criteria

### Table View Expansion
- [ ] Clicking backlog column header expands to full-width table view
- [ ] Table shows two columns: Prompt (wide, left) and Priority (narrow, right)
- [ ] Click header again or "X" button to collapse back to kanban column

### Prompt Column (Left)
- [ ] Shows full prompt text, multiline (no truncation)
- [ ] Click to edit inline (textarea)
- [ ] Changes trigger API update via `v1SpecTasksUpdate`
- [ ] Show loading indicator during save
- [ ] Show error toast if update fails

### Priority Column (Right)
- [ ] Shows priority as colored chip/dropdown
- [ ] Click to show dropdown (Critical, High, Medium, Low)
- [ ] Change triggers immediate API update
- [ ] Table re-renders with new sort order after change

### Auto-Sort Behavior
- [ ] Tasks sort by priority: critical (top) → high → medium → low (bottom)
- [ ] Secondary sort by created date (newest first) within same priority

### Filter Bar
- [ ] Text search filters by prompt content
- [ ] Priority filter dropdown (multi-select)
- [ ] Clear filters button when any filter is active