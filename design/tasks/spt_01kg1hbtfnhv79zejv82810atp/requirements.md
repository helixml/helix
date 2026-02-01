# Requirements: Backlog Table View

## Overview
When users have many tasks in the backlog, they need a way to view all prompts at once and reorder them by priority.

## User Stories

### US-1: View Backlog as Table
**As a** project manager  
**I want to** expand the backlog into a table view  
**So that** I can see all pending tasks at a glance without scrolling through cards

### US-2: Reorder Tasks by Priority
**As a** project manager  
**I want to** drag-and-drop tasks to reorder them  
**So that** I can prioritize which tasks should be worked on first

### US-3: View Full Prompts
**As a** user  
**I want to** see the full prompt text for each backlog task  
**So that** I can understand what each task involves before prioritizing

## Acceptance Criteria

### AC-1: Table View Toggle
- [ ] Button in the backlog column header to toggle table view
- [ ] Table view shows as a modal/overlay (similar to audit trail)
- [ ] Can close table view to return to kanban

### AC-2: Table Columns
- [ ] Task number (#00001 format)
- [ ] Name/Title
- [ ] Full prompt (expandable if long)
- [ ] Priority (low/medium/high/critical)
- [ ] Created date
- [ ] Status (backlog, queued, failed)

### AC-3: Drag-and-Drop Reordering
- [ ] Rows can be dragged to reorder
- [ ] Reorder persists to backend (new `sort_order` field)
- [ ] Visual feedback during drag

### AC-4: Priority Quick-Edit
- [ ] Click priority chip to change priority inline
- [ ] Priority changes persist immediately

## Out of Scope
- Bulk actions (select multiple)
- Filtering within table (use existing filters)
- Sorting by columns (manual order is the point)