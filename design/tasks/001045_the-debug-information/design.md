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