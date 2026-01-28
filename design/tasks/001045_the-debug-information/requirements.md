# Requirements: Debug Information Panel & Task Number Display

## User Stories

### US1: Git Branch & Folder in Debug Panel
**As a** developer debugging spec tasks  
**I want** to see the Git branch name and helix-specs folder path in the debug panel  
**So that** I can quickly navigate to the right files and branches without looking them up

### US2: Task Number on Task Cards
**As a** user viewing the Kanban board  
**I want** to see the task number displayed on each task card  
**So that** I can easily reference and communicate about specific tasks

### US3: Task Number in Split Screen Tabs
**As a** user working in split screen mode  
**I want** to see the task number on the left side of each tab  
**So that** I can quickly identify which task I'm viewing when multiple tabs are open

## Acceptance Criteria

### Debug Information Panel (SpecTaskDetailContent)
- [ ] Display Git branch name (already shown, verify it's the `branch_name` field)
- [ ] Display helix-specs folder path from `design_doc_path` field (e.g., "000045_the-debug-information")
- [ ] Both fields should use monospace font consistent with existing debug info styling
- [ ] Show "N/A" if fields are not yet assigned

### Task Card (TaskCard component)
- [ ] Display task number in bottom-right corner of card
- [ ] Format as zero-padded 6-digit number (e.g., "#001045")
- [ ] Use subtle styling (small font, muted color) so it doesn't dominate the card
- [ ] Only show if `task_number` field is set (> 0)

### Split Screen Tabs (PanelTab in TabsView)
- [ ] Display task number on left side of tab, before the title
- [ ] Format as "#NNNNNN" (e.g., "#001045")
- [ ] Use same color as tab text (primary when active, secondary when inactive)
- [ ] Only show for task tabs (not desktop, review, or create tabs)
- [ ] Only show if `task_number` field is set

## Data Sources

All required data already exists in the API:
- `TypesSpecTask.task_number` - Auto-assigned unique number
- `TypesSpecTask.design_doc_path` - Folder name in helix-specs (e.g., "000045_the-debug-information")
- `TypesSpecTask.branch_name` - Git branch name (already displayed in debug panel)