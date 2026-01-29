# Requirements: Kanban Board in Split View

## Overview
Allow users to view the Kanban board as a tab within the split-screen workspace, enabling side-by-side viewing of the board and task details.

## User Stories

### US1: View Kanban as a Tab
**As a** project manager  
**I want** to open the Kanban board in the split view workspace  
**So that** I can see the board alongside task details without switching views

### US2: Open Tasks from Embedded Kanban
**As a** user  
**I want** clicking a task in the embedded Kanban to open it in a split pane  
**So that** I can keep the Kanban visible while viewing task details

## Acceptance Criteria

### AC1: Kanban Tab Type
- [ ] Add "kanban" as a new tab type in `TabsView`
- [ ] Kanban tab appears in the tab bar like other tabs (tasks, reviews, desktops)
- [ ] Kanban tab can be closed, dragged, and split like other tabs

### AC2: Add Kanban Button
- [ ] Add a button/option in the workspace toolbar to open Kanban as a tab
- [ ] If Kanban tab already exists, switch to it instead of creating duplicate

### AC3: Task Click Behavior
- [ ] Clicking a task in the embedded Kanban opens it in a new/existing split pane
- [ ] The Kanban tab remains open (not replaced)
- [ ] If task is already open in another tab, switch to that tab

### AC4: Visual Consistency
- [ ] Kanban in split view uses same styling as full-page Kanban
- [ ] Responsive layout works in narrower split pane widths

## Out of Scope
- Drag-and-drop between Kanban columns in split view (use existing behavior)
- Multiple Kanban tabs for different projects (one Kanban per workspace)