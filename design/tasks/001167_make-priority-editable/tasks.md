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
- [ ] Create MUI Table with columns: Name, Priority, Type, Description, Created
- [ ] Sort tasks by priority (critical → high → medium → low) then by created date
- [ ] Truncate description column to ~50 chars with ellipsis
- [ ] Format created date as relative time ("2h ago", "3d ago")
- [ ] Add row hover state for better UX

## Inline Priority Editing
- [ ] Render priority cell as clickable chip/button
- [ ] On click, show MUI Select dropdown with priority options
- [ ] Use `TypesSpecTaskPriority` enum values (Critical, High, Medium, Low)
- [ ] Apply priority colors matching existing `getPriorityColor()` function
- [ ] Call `useUpdateSpecTask` mutation on selection change
- [ ] Show loading spinner in cell during API call
- [ ] Show error snackbar on failure
- [ ] Animate row position change after successful update

## BacklogFilterBar Component
- [ ] Add text input with search icon for filtering by name/description
- [ ] Add priority multi-select dropdown filter
- [ ] Add "Clear filters" button (visible only when filters active)
- [ ] Wire filter state to parent component via props
- [ ] Apply filters to task list before rendering table

## Integration
- [ ] Import and wire up `BacklogFilterBar` in `BacklogTableView`
- [ ] Add filter state (`search`, `priorities[]`) to `SpecTaskKanbanBoard`
- [ ] Pass filtered tasks to `BacklogTableView`
- [ ] Test priority update triggers re-sort
- [ ] Verify kanban column task count updates after table edits

## Polish
- [ ] Add keyboard navigation (Escape to close dropdown)
- [ ] Add empty state when no tasks match filters
- [ ] Test light/dark theme compatibility
- [ ] Ensure responsive layout on smaller screens