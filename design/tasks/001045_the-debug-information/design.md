# Design: Debug Information Panel & Task Number Display

## Overview

This is a small UI enhancement task that adds visibility for task numbers and helix-specs folder paths in three locations:
1. Debug information panel in SpecTaskDetailContent
2. Bottom-right corner of TaskCard components
3. Left side of tabs in TabsView split screen

## Existing Patterns

### Data Already Available
The API already provides all required fields on `TypesSpecTask`:
- `task_number: number` - Auto-assigned globally unique number (e.g., 1045)
- `design_doc_path: string` - Folder name in helix-specs (e.g., "001045_the-debug-information")
- `branch_name: string` - Git branch name (already shown in debug panel)

### Current Debug Panel Location
The debug panel is in `SpecTaskDetailContent.tsx` around line 960-1050. It already shows:
- Task ID
- Branch name (with PUSH indicator)
- Base branch (with PULL indicator)
- Session ID
- Desktop version
- GPU info

### Task Number Formatting Convention
From `ProjectAuditTrail.tsx` line 314-318, task numbers are displayed as:
```tsx
`#${String(log.metadata.task_number).padStart(6, '0')}`
```
This produces "#001045" format.

## Implementation Approach

### 1. Debug Panel Enhancement
Add two new fields after the Branch display:
- **Specs Folder**: Shows `design_doc_path` value
- Keep same monospace styling as other debug fields

### 2. TaskCard Task Number
Add a small chip/badge in bottom-right of the card:
- Position: `position: 'absolute', bottom: 8, right: 8`
- Style: Small muted text, slightly transparent
- Conditional: Only render if `task.task_number > 0`

### 3. TabsView Tab Number
Modify `PanelTab` component to prefix task number:
- Add before the `displayTitle` Typography
- Same text styling as title (inherits active/inactive colors)
- Conditional: Only for `tab.type === 'task'` and `displayTask?.task_number > 0`

## Component Changes

| File | Change |
|------|--------|
| `SpecTaskDetailContent.tsx` | Add design_doc_path to debug panel |
| `TaskCard.tsx` | Add task number badge in bottom-right |
| `TabsView.tsx` | Prefix task number in PanelTab |

## Risks & Mitigations

**Risk**: Task number might not be assigned yet for new/backlog tasks
**Mitigation**: Conditional rendering - only show when `task_number > 0`

**Risk**: Long task numbers could overflow
**Mitigation**: 6-digit format is fixed width, won't grow unexpectedly

## Implementation Notes

### Files Modified
1. **`SpecTaskDetailContent.tsx`** (~line 979-1029)
   - Added Task # field right after Task ID
   - Added Specs Folder field after Base Branch
   - Both use same monospace styling as existing debug fields
   - Show "N/A" when fields not set

2. **`TaskCard.tsx`**
   - Added `task_number?: number` to `SpecTaskWithExtras` interface
   - Added Typography badge after CardContent, before CloneTaskDialog
   - Positioned absolutely at bottom-right (bottom: 8, right: 8)
   - Style: 0.65rem monospace, text.disabled color, 0.7 opacity

3. **`TabsView.tsx`** (PanelTab component ~line 600-660)
   - Added task number Typography before the displayTitle
   - Only renders for `tab.type === 'task'` when `displayTask?.task_number > 0`
   - Inherits same color styling as title (active/inactive)
   - Slight opacity (0.7) to not compete with title

### Gotchas
- ~~Task numbers are only assigned when planning starts, so backlog tasks won't have them~~ **FIXED**: Now assigned at creation
- The `task_number` field needed to be added to `SpecTaskWithExtras` interface in TaskCard.tsx
- Edit tool reformatted both files (prettier-style) - this is fine, the important changes are there

### Backend: Task Number Assignment at Creation (Added)

User requested that task numbers be assigned immediately when tasks are created, not when planning starts.

**Files Modified:**

1. **`api/pkg/services/spec_driven_task_service.go`** (CreateTaskFromPrompt)
   - Added call to `IncrementGlobalTaskNumber()` right after task struct creation
   - Generate `DesignDocPath` using `GenerateDesignDocPath(task, taskNumber)`
   - Existing code in `StartSpecGeneration` already has `if task.TaskNumber == 0` guard

2. **`api/pkg/server/spec_task_clone_handlers.go`** (cloneTaskToProject)
   - Added task number assignment before `CreateSpecTask` call
   - Import added for `services` package to use `GenerateDesignDocPath`

3. **`api/pkg/agent/skill/project/spec_task_create_tool.go`** (CreateSpecTaskTool.Execute)
   - Added task number assignment when agent creates tasks via tool
   - Import added for `services` package

4. **`api/pkg/server/simple_sample_projects.go`** (forkSimpleProject)
   - Added task number assignment in both task creation loops (clone demo tasks + sample project tasks)
   - `services` package already imported

**Pattern used in all locations:**
```go
taskNumber, err := s.store.IncrementGlobalTaskNumber(ctx)
if err != nil {
    log.Warn().Err(err).Msg("Failed to get global task number, using fallback")
    taskNumber = 1
}
task.TaskNumber = taskNumber
task.DesignDocPath = services.GenerateDesignDocPath(task, taskNumber)
```