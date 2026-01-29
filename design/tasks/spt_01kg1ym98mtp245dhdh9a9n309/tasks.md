# Implementation Tasks

## Core Tab Type Support

- [ ] Add `"kanban"` to the `TabData.type` union in `TabsView.tsx`
- [ ] Add Kanban rendering case in `TaskPanel` content area (around L1345-1447)
- [ ] Pass `projectId` and `onTaskClick` props to embedded `SpecTaskKanbanBoard`

## Add Kanban Button to Workspace

- [ ] Add `handleAddKanban(panelId)` function in `TabsView.tsx`
  - Check if kanban tab already exists → activate it
  - Otherwise create new kanban tab: `{ id: 'kanban', type: 'kanban' }`
- [ ] Add Kanban icon button to workspace toolbar (use `Kanban` from lucide-react)
- [ ] Wire button to call `handleAddKanban` for the current panel

## Task Click Behavior

- [ ] Implement `onTaskClick` handler for embedded Kanban:
  - Check if task tab already open in any leaf → activate it
  - Otherwise create new task tab in adjacent split pane
- [ ] Use existing `handleSplitPanel` pattern to create vertical split (Kanban left, task right)

## Persistence

- [ ] Update `serializeNode` to handle kanban tabs: `"kanban:${tab.id}"`
- [ ] Update `deserializeNode` to restore kanban tabs from serialized format
- [ ] Test that kanban tab survives page refresh

## Testing

- [ ] Verify Kanban tab opens from workspace toolbar button
- [ ] Verify clicking task in embedded Kanban opens task in split
- [ ] Verify Kanban tab can be closed, dragged, and split like other tabs
- [ ] Verify only one Kanban tab can exist at a time (no duplicates)
- [ ] Verify layout works at various split pane widths