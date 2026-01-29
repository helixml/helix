# Implementation Tasks

## Feature 1: Prompt Tooltip on Task Cards

- [ ] Import MUI `Tooltip` in `TaskCard.tsx`
- [ ] Wrap the `<Card>` component with `<Tooltip>` showing `task.original_prompt || task.description || task.name`
- [ ] Configure tooltip: `enterDelay={300}`, `placement="right"`, max-height with scroll for long content
- [ ] Test tooltip appears on all task cards (backlog, planning, review, etc.)

## Feature 2: Backlog Table View

- [ ] Add `backlogExpanded` state to `SpecTaskKanbanBoard.tsx`
- [ ] Make "Backlog" column header clickable with visual indicator (chevron icon)
- [ ] Create `BacklogTableView` component with MUI Table (columns: drag handle, name, prompt, priority, created)
- [ ] Conditionally render table view vs column based on `backlogExpanded` state
- [ ] Wire up dnd-kit for row reordering (`DndContext`, `SortableContext`, `useSortable`)
- [ ] Store reorder in component state (optimistic UI)

## Feature 2b: Persist Reorder (Optional, can defer)

- [ ] Add `SortOrder int` field to `SpecTask` in `simple_spec_task.go`
- [ ] Add database migration for sort_order column
- [ ] Add `PATCH /api/v1/spec-tasks/{id}/reorder` endpoint
- [ ] Call API on drag end to persist order
- [ ] Update `useSpecTasks` query to sort by `sort_order`

## Testing & Polish

- [ ] Verify tooltip doesn't interfere with card click handler
- [ ] Test table view on mobile/small screens (hide or responsive)
- [ ] Run `yarn test && yarn build` before committing