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
- [x] Modify `PanelTab` component in `TabsView.tsx` to show task number
- [x] Add before displayTitle, format as "#001045 "
- [x] Use same color styling as tab title (inherits active/inactive state)
- [x] Only show for task tabs (`tab.type === 'task'`) when `displayTask?.task_number > 0`

## Backend: Assign Task Numbers at Creation
- [x] Move task number assignment from `StartSpecGeneration` to `CreateTaskFromPrompt` in `spec_driven_task_service.go`
- [x] Also update `cloneTaskToProject` in `spec_task_clone_handlers.go` to assign task number at clone time
- [x] Also update `CreateSpecTaskTool` agent tool in `spec_task_create_tool.go`
- [x] Also update sample project task creation in `simple_sample_projects.go`
- [x] Ensure design_doc_path is also generated at creation time (not just at planning start)

## Testing
- [x] Verify task number appears on cards in Kanban board
- [x] Verify task number appears in split screen tabs
- [x] Verify debug panel shows specs folder path
- [x] Verify fields gracefully handle missing data (new tasks without numbers)
- [ ] Verify new tasks get task numbers immediately when created