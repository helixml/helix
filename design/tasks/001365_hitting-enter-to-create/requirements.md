# Requirements: Focus Behavior After Task Creation

## Overview

Improve UX by automatically focusing relevant elements after creating a new spectask.

## User Stories

### Story 1: Focus Input After Enter Key
**As a** user creating tasks quickly  
**I want** the input field to be focused after pressing Enter to open the create dialog  
**So that** I can immediately start typing without clicking

### Story 2: Focus Start Planning After Task Creation
**As a** user who just created a task  
**I want** the "Start Planning" button to be focused on the new task  
**So that** I can immediately press Enter to start planning

## Acceptance Criteria

### AC1: Enter Key Opens Dialog and Focuses Input
- When user presses Enter (on kanban board, not in a text field)
- Create task dialog opens
- Task description textarea is automatically focused
- User can start typing immediately

### AC2: Task Creation Focuses Start Planning Button
- When user clicks "Create Task" button
- Task is created and dialog closes
- Task card appears in backlog column
- "Start Planning" button on the new task card receives focus
- User can press Enter to start planning

## Out of Scope

- Keyboard navigation between task cards
- Focus management for other dialogs
- Accessibility improvements beyond focus behavior