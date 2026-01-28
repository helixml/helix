# Implementation Tasks

## Debug Panel Enhancement
- [x] Add design_doc_path display to debug panel in `SpecTaskDetailContent.tsx` (~line 990, after branch_name display)
- [x] Label it "Specs Folder:" with same monospace styling as other debug fields
- [x] Show "N/A" if design_doc_path is not set

## Task Card Task Number
- [x] Add task number badge to `TaskCard.tsx` in bottom-right corner of card
- [x] Format as "#001045" using `String(task.task_number).padStart(6, '0')`
- [x] Style: small font (0.65rem), muted color (text.disabled), absolute positioned
- [x] Only render when `task.task_number > 0`

## Split Screen Tabs Task Number
- [~] Modify `PanelTab` component in `TabsView.tsx` to show task number
- [~] Add before displayTitle, format as "#001045 "
- [~] Use same color styling as tab title (inherits active/inactive state)
- [~] Only show for task tabs (`tab.type === 'task'`) when `displayTask?.task_number > 0`

## Testing
- [ ] Verify task number appears on cards in Kanban board
- [ ] Verify task number appears in split screen tabs
- [ ] Verify debug panel shows specs folder path
- [ ] Verify fields gracefully handle missing data (new tasks without numbers)