# Requirements: Backlog Prompt Tooltip & Table View

## Overview

Improve backlog task visibility by allowing users to see full prompts on hover and expand the backlog into a table view for easier review and reordering.

## User Stories

### US1: Hover to View Full Prompt
**As a** user reviewing tasks  
**I want to** hover over any task card and see the full prompt  
**So that** I can quickly understand what each task is about without clicking into it

**Acceptance Criteria:**
- [ ] Hovering over a task card shows a tooltip/popover with the full `original_prompt` text
- [ ] Tooltip appears after a short delay (~300ms) to avoid accidental triggers
- [ ] Tooltip disappears when mouse leaves the card
- [ ] Long prompts are scrollable within the tooltip (max height ~300px)
- [ ] Works on all task cards, not just backlog

### US2: Backlog Table View
**As a** user managing multiple backlog tasks  
**I want to** click on the "Backlog" column header to expand into a table view  
**So that** I can see all prompts at once and easily reorder tasks

**Acceptance Criteria:**
- [ ] Clicking the "Backlog" column header expands to a full-width table view
- [ ] Table shows: task name, full prompt (or truncated with expand), priority, created date
- [ ] Rows are draggable for reordering (using existing dnd-kit patterns)
- [ ] Clicking header again collapses back to Kanban column view
- [ ] Visual indicator (icon/chevron) shows the header is clickable
- [ ] Reordering persists (requires new `sort_order` field on SpecTask)

## Out of Scope

- Editing prompts inline in table view (use existing detail view)
- Filtering/searching in table view (future enhancement)
- Table view for other columns (only backlog for now)