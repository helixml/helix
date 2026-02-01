# Design: Backlog Table View with Editable Priority

## Overview

Add an expandable table view to the backlog column in `SpecTaskKanbanBoard.tsx`. When clicked, the backlog header expands to show a full-width table with inline priority editing and filtering capabilities.

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
        └── BacklogTable (using MUI Table, not DataGrid)
```

### State Management

Add to `SpecTaskKanbanBoard`:
```typescript
const [backlogExpanded, setBacklogExpanded] = useState(false)
const [backlogFilters, setBacklogFilters] = useState({
  search: '',
  priorities: [] as TypesSpecTaskPriority[]
})
```

### Key Decisions

1. **MUI Table vs DataGrid**: Use MUI Table components for simplicity. The existing `DataGrid` uses a third-party library (`@inovua/reactdatagrid-community`) which is overkill for this use case.

2. **Inline Edit Pattern**: Use a `Select` dropdown that appears on cell click, matching the existing priority edit UI in `SpecTaskDetailContent.tsx`.

3. **Sorting**: Client-side sorting using a priority weight map:
   ```typescript
   const PRIORITY_ORDER = { critical: 0, high: 1, medium: 2, low: 3 }
   ```

4. **API Integration**: Reuse `useUpdateSpecTask` hook from `services/specTaskService.ts` for priority updates.

5. **Animation**: Use CSS transitions for row reordering after priority change (simple fade/slide).

## Data Flow

```
User clicks priority cell
  → Opens Select dropdown
  → User selects new priority
  → Call updateSpecTask({ taskId, updates: { priority } })
  → On success: invalidate 'specTasks' query (auto-refetch)
  → Table re-renders with new sort order
```

## UI Layout (Expanded State)

```
┌─────────────────────────────────────────────────────────────────────┐
│ Backlog (12)                                              [X Close] │
├─────────────────────────────────────────────────────────────────────┤
│ [🔍 Search tasks...        ] [Priority ▼] [Clear filters]           │
├─────────────────────────────────────────────────────────────────────┤
│ Name              │ Priority │ Type    │ Description       │ Created│
├─────────────────────────────────────────────────────────────────────┤
│ Fix login bug     │ Critical▼│ bug     │ Users cannot...   │ 2h ago │
│ Add dark mode     │ High    ▼│ feature │ Implement theme...│ 1d ago │
│ Update docs       │ Medium  ▼│ task    │ Add API docs...   │ 3d ago │
└─────────────────────────────────────────────────────────────────────┘
```

## Files to Modify/Create

| File | Action | Description |
|------|--------|-------------|
| `frontend/src/components/tasks/BacklogTableView.tsx` | Create | New table component |
| `frontend/src/components/tasks/BacklogFilterBar.tsx` | Create | Filter bar component |
| `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx` | Modify | Add expansion state and render BacklogTableView |

## Existing Patterns Used

- Priority dropdown: `SpecTaskDetailContent.tsx` lines 898-919
- Task update hook: `useUpdateSpecTask` from `services/specTaskService.ts`
- Priority colors: `getPriorityColor()` in `SpecTaskDetailContent.tsx`
- Column header styling: `DroppableColumn` in `SpecTaskKanbanBoard.tsx` lines 244-294