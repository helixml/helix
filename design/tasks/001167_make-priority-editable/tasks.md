# Implementation Tasks

## Setup
- [x] Create `frontend/src/components/tasks/BacklogTableView.tsx` skeleton component
- [x] Create `frontend/src/components/tasks/BacklogFilterBar.tsx` skeleton component

## Backlog Expansion
- [x] Add `backlogExpanded` state to `SpecTaskKanbanBoard.tsx`
- [x] Make backlog column header clickable (cursor pointer, onClick handler)
- [x] Conditionally render `BacklogTableView` when expanded instead of `DroppableColumn`
- [x] Add close button (X) to collapse back to kanban view
- [x] Style expanded view to span full width of kanban board area

## BacklogTableView Component
- [x] Create MUI Table with two columns: Prompt (wide) and Priority (narrow)
- [x] Display full prompt text multiline (no truncation)
- [x] Sort tasks by priority (critical → high → medium → low) then by created date
- [x] Add row hover state for better UX

## Inline Editing
- [x] **Prompt cell**: Click to edit in-place with textarea, blur/Enter to save
- [x] **Priority cell**: Click to show Select dropdown with colored options
- [x] Apply priority colors matching existing `getPriorityColor()` function
- [x] Call `useUpdateSpecTask` mutation on change
- [x] Show loading indicator during API call
- [x] Show error snackbar on failure

## BacklogFilterBar Component
- [x] Add text input with search icon for filtering by prompt content
- [x] Add priority multi-select dropdown filter
- [x] Add "Clear" button (visible only when filters active)
- [x] Wire filter state to parent component via props
- [x] Apply filters to task list before rendering table

## Integration
- [x] Import and wire up `BacklogFilterBar` in `BacklogTableView`
- [x] Add filter state (`search`, `priorities[]`) to `SpecTaskKanbanBoard`
- [x] Pass filtered tasks to `BacklogTableView`
- [ ] Test that priority edits trigger re-sort
- [ ] Verify kanban column task count updates after table edits

## Polish
- [x] Add keyboard navigation (Escape to cancel edit, Enter to save)
- [x] Add empty state when no tasks match filters
- [ ] Test light/dark theme compatibility