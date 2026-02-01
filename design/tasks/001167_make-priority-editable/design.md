# Design: Backlog Table View with Editable Fields

## Overview

Add an expandable table view to the backlog column in `SpecTaskKanbanBoard.tsx`. When clicked, the backlog header expands to show a full-width table with inline editing for all fields (name, priority, type, prompt) and filtering capabilities.

## Architecture

### Component Structure

```
SpecTaskKanbanBoard.tsx
├── DroppableColumn (existing - unchanged for non-backlog columns)
└── BacklogColumn (new wrapper)
    ├── BacklogColumnHeader (clickable to expand/collapse)
    ├── DroppableColumn (collapsed state - existing)
    └── BacklogTableView (expanded state - new)
        ├── BacklogFilterBar
        └── BacklogTable (using MUI Table)
            └── BacklogTableRow (expandable for full prompt editing)
```

### State Management

Add to `SpecTaskKanbanBoard`:
```typescript
const [backlogExpanded, setBacklogExpanded] = useState(false)
const [backlogFilters, setBacklogFilters] = useState({
  search: '',
  priorities: [] as TypesSpecTaskPriority[]
})
const [expandedRowId, setExpandedRowId] = useState<string | null>(null)
```

### Key Decisions

1. **MUI Table vs DataGrid**: Use MUI Table components for simplicity. The existing `DataGrid` uses a third-party library (`@inovua/reactdatagrid-community`) which is overkill for this use case.

2. **Inline Edit Pattern**: 
   - Name/Type: Click cell to switch to edit mode (text input / select)
   - Priority: Click to show dropdown, matching existing UI in `SpecTaskDetailContent.tsx`
   - Prompt: Click row to expand and show full textarea below

3. **Expandable Row for Prompt**: Since prompts can be long, clicking the prompt cell expands the row to show a full-height textarea below the main row content.

4. **Sorting**: Client-side sorting using a priority weight map:
   ```typescript
   const PRIORITY_ORDER = { critical: 0, high: 1, medium: 2, low: 3 }
   ```
   No animation on re-sort - table simply re-renders with new order.

5. **API Integration**: Reuse `useUpdateSpecTask` hook from `services/specTaskService.ts` for all field updates.

## Data Flow

```
User clicks cell to edit
  → Cell enters edit mode (input/select/textarea)
  → User makes change and blurs or presses Enter
  → Call updateSpecTask({ taskId, updates: { [field]: value } })
  → Show loading indicator in cell
  → On success: invalidate 'specTasks' query (auto-refetch)
  → Table re-renders with updated data and new sort order
```

## UI Layout (Expanded State)

```
┌─────────────────────────────────────────────────────────────────────────┐
│ Backlog (12)                                                  [X Close] │
├─────────────────────────────────────────────────────────────────────────┤
│ [🔍 Search tasks...        ] [Priority ▼] [Clear filters]               │
├─────────────────────────────────────────────────────────────────────────┤
│ Name              │ Priority │ Type    │ Prompt              │ Created  │
├─────────────────────────────────────────────────────────────────────────┤
│ Fix login bug     │ Critical▼│ bug    ▼│ Users cannot log... │ 2h ago   │
├─────────────────────────────────────────────────────────────────────────┤
│ Add dark mode     │ High    ▼│ feature▼│ Implement theme...  │ 1d ago   │
│ ┌─────────────────────────────────────────────────────────────────────┐ │
│ │ Full prompt textarea (expanded):                                    │ │
│ │ Implement a dark/light mode toggle with persistent user preference. │ │
│ │ Should respect system preference by default.                        │ │
│ │                                              [Cancel] [Save]        │ │
│ └─────────────────────────────────────────────────────────────────────┘ │
├─────────────────────────────────────────────────────────────────────────┤
│ Update docs       │ Medium  ▼│ task   ▼│ Add API docs for... │ 3d ago   │
└─────────────────────────────────────────────────────────────────────────┘
```

## Editable Cell Behaviors

| Column | Display | Edit Mode | Trigger |
|--------|---------|-----------|---------|
| Name | Text | TextField | Click |
| Priority | Colored chip | Select dropdown | Click |
| Type | Text | Select dropdown | Click |
| Prompt | Truncated (~100 chars) | Expanded textarea below row | Click |
| Created | Relative time | Not editable | - |

## Files to Modify/Create

| File | Action | Description |
|------|--------|-------------|
| `frontend/src/components/tasks/BacklogTableView.tsx` | Create | Main table component with expandable rows |
| `frontend/src/components/tasks/BacklogFilterBar.tsx` | Create | Filter bar component |
| `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx` | Modify | Add expansion state and render BacklogTableView |

## Existing Patterns Used

- Priority dropdown: `SpecTaskDetailContent.tsx` lines 898-919
- Task update hook: `useUpdateSpecTask` from `services/specTaskService.ts`
- Priority colors: `getPriorityColor()` in `SpecTaskDetailContent.tsx`
- Column header styling: `DroppableColumn` in `SpecTaskKanbanBoard.tsx` lines 244-294
- Inline text editing: Standard MUI TextField with onBlur save pattern