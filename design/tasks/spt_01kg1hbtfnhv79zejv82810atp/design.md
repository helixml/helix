# Design: Backlog Table View

## Architecture

### Component Location
New component: `frontend/src/components/tasks/BacklogTableView.tsx`

This follows the existing pattern of task-related components in `components/tasks/`.

### Existing Patterns Used

1. **Table Layout**: Follow `ProjectAuditTrail.tsx` pattern - MUI Table with sticky header, expandable rows, pagination
2. **Drag-and-Drop**: Use `@dnd-kit` (already in codebase) - see `RobustPromptInput.tsx` for sortable list pattern
3. **Modal Overlay**: Use MUI Dialog, similar to other modal patterns in the app

## Data Model Changes

### New Field: `sort_order` on SpecTask
```go
// In api/pkg/types/simple_spec_task.go
SortOrder int `json:"sort_order" gorm:"default:0;index"` // Lower = higher priority in backlog
```

### API Endpoint
```
PATCH /api/v1/spec-tasks/{id}/reorder
Body: { "sort_order": 5 }
```

Or batch reorder:
```
POST /api/v1/projects/{id}/spec-tasks/reorder
Body: { "task_ids": ["id1", "id2", "id3"] }  // Order determines sort_order
```

## UI Design

### Toggle Button
- Icon button in backlog column header (table icon)
- Tooltip: "View as table"

### Table Dialog
- Full-screen dialog (like audit trail)
- Title: "Backlog Tasks"
- Close button returns to kanban

### Table Rows
- Drag handle on left
- Task number (#00001)
- Name (truncated, tooltip for full)
- Prompt (expandable row on click)
- Priority (clickable chip → dropdown)
- Created date
- Status chip (backlog/queued/failed)

### Drag Behavior
- Row highlights during drag
- Drop indicator between rows
- On drop: call reorder API with new positions

## Key Decisions

1. **Dialog vs Inline**: Using dialog overlay because:
   - Doesn't disrupt kanban layout
   - Full width for table
   - Clear exit path back to board

2. **Sort Order vs Priority**: These are separate concerns:
   - Priority: urgency/importance label
   - Sort Order: actual queue position for processing
   - Users can have high-priority tasks lower in queue

3. **Batch Reorder API**: Single API call on drag-end rather than per-row updates - reduces API calls and ensures atomic ordering