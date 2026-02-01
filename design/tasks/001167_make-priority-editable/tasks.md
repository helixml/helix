# Implementation Tasks

## Setup
- [ ] Create `frontend/src/components/tasks/BacklogTableView.tsx` skeleton component
- [ ] Create `frontend/src/components/tasks/BacklogFilterBar.tsx` skeleton component

## Backlog Expansion
- [ ] Add `backlogExpanded` state to `SpecTaskKanbanBoard.tsx`
- [ ] Make backlog column header clickable (cursor pointer, onClick handler)
- [ ] Conditionally render `BacklogTableView` when expanded instead of `DroppableColumn`
- [ ] Add close button (X) to collapse back to kanban view
- [ ] Style expanded view to span full width of kanban board area

## BacklogTableView Component
- [ ] Create MUI Table with columns: Name, Priority, Type, Prompt, Created
- [ ] Sort tasks by priority (critical → high → medium → low) then by created date
- [ ] Truncate prompt column to ~100 chars with ellipsis
- [ ] Format created date as relative time ("2h ago", "3d ago")
- [ ] Add row hover state for better UX
- [ ] Track `expandedRowId` state for prompt expansion

## Inline Editing - All Cells
- [ ] **Name cell**: Click to show TextField, blur/Enter to save
- [ ] **Priority cell**: Click to show Select dropdown with colored options
- [ ] **Type cell**: Click to show Select dropdown (feature, bug, task, epic)
- [ ] Apply priority colors matching existing `getPriorityColor()` function
- [ ] Call `useUpdateSpecTask` mutation on field change
- [ ] Show loading spinner in cell during API call
- [ ] Show error snackbar on failure

## Expandable Prompt Row
- [ ] Click prompt cell to expand row and show full prompt textarea
- [ ] Textarea fills width below the row content
- [ ] Add Save and Cancel buttons below textarea
- [ ] Save calls `useUpdateSpecTask` with updated prompt
- [ ] Cancel collapses row without saving
- [ ] Only one row can be expanded at a time

## BacklogFilterBar Component
- [ ] Add text input with search icon for filtering by name/prompt
- [ ] Add priority multi-select dropdown filter
- [ ] Add "Clear filters" button (visible only when filters active)
- [ ] Wire filter state to parent component via props
- [ ] Apply filters to task list before rendering table

## Integration
- [ ] Import and wire up `BacklogFilterBar` in `BacklogTableView`
- [ ] Add filter state (`search`, `priorities[]`) to `SpecTaskKanbanBoard`
- [ ] Pass filtered tasks to `BacklogTableView`
- [ ] Test that edits trigger re-sort (no animation, just re-render)
- [ ] Verify kanban column task count updates after table edits

## Polish
- [ ] Add keyboard navigation (Escape to cancel edit, Enter to save)
- [ ] Add empty state when no tasks match filters
- [ ] Test light/dark theme compatibility
- [ ] Ensure responsive layout on smaller screens