# Design: Backlog Table View with Editable Priority

## Overview

Add an expandable table view to the backlog column in `SpecTaskKanbanBoard.tsx`. When clicked, the backlog header expands to show a full-width table with two columns: Prompt (full text, multiline) and Priority (editable dropdown).

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

1. **Two columns only**: Prompt (wide, left) and Priority (narrow, right). No name, type, or created columns - spec tasks are defined by their prompt.

2. **Full prompt display**: Show entire prompt text multiline in each row. No truncation - users want to read all prompts in a stack.

3. **Inline editing**: 
   - Prompt: Click to edit in-place (textarea)
   - Priority: Click to show dropdown

4. **Sorting**: Client-side sorting using a priority weight map:
   ```typescript
   const PRIORITY_ORDER = { critical: 0, high: 1, medium: 2, low: 3 }
   ```

5. **API Integration**: Reuse `useUpdateSpecTask` hook from `services/specTaskService.ts`.

## Data Flow

```
User edits prompt or changes priority
  → Call updateSpecTask({ taskId, updates: { original_prompt } }) or { priority }
  → Show loading indicator
  → On success: invalidate 'specTasks' query (auto-refetch)
  → Table re-renders with new sort order (if priority changed)
```

## UI Layout (Expanded State)

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ Backlog (12)                                                       [X Close] │
├──────────────────────────────────────────────────────────────────────────────┤
│ [🔍 Search prompts...      ] [Priority ▼] [Clear]                            │
├──────────────────────────────────────────────────────────────────────────────┤
│ Prompt                                                           │ Priority  │
├──────────────────────────────────────────────────────────────────────────────┤
│ Fix the login bug where users cannot authenticate when using     │ Critical▼ │
│ SSO with Azure AD. The error occurs after the redirect callback. │           │
├──────────────────────────────────────────────────────────────────────────────┤
│ Implement a dark/light mode toggle with persistent user          │ High    ▼ │
│ preference. Should respect system preference by default.         │           │
├──────────────────────────────────────────────────────────────────────────────┤
│ Add API documentation for the new /v2/sessions endpoints.        │ Medium  ▼ │
├──────────────────────────────────────────────────────────────────────────────┤
│ Consider adding keyboard shortcuts for common actions.           │ Low     ▼ │
└──────────────────────────────────────────────────────────────────────────────┘
```

## Column Specifications

| Column | Width | Display | Edit Mode |
|--------|-------|---------|-----------|
| Prompt | ~85% (flex grow) | Full multiline text | Textarea (click to edit) |
| Priority | ~15% (fixed ~100px) | Colored chip | Select dropdown |

## Files to Modify/Create

| File | Action | Description |
|------|--------|-------------|
| `frontend/src/components/tasks/BacklogTableView.tsx` | Create | Table with prompt + priority columns |
| `frontend/src/components/tasks/BacklogFilterBar.tsx` | Create | Search and priority filter |
| `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx` | Modify | Add expansion state, render BacklogTableView |

## Existing Patterns Used

- Priority dropdown: `SpecTaskDetailContent.tsx` lines 898-919
- Task update hook: `useUpdateSpecTask` from `services/specTaskService.ts`
- Priority colors: `getPriorityColor()` in `SpecTaskDetailContent.tsx`
- Column header styling: `DroppableColumn` in `SpecTaskKanbanBoard.tsx` lines 244-294

## Implementation Notes

### Files Created
- `frontend/src/components/tasks/BacklogTableView.tsx` - Main table component (354 lines)
- `frontend/src/components/tasks/BacklogFilterBar.tsx` - Filter bar component (133 lines)

### Files Modified
- `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx` - Added:
  - `backlogExpanded` state (line 467)
  - `onHeaderClick` prop to `DroppableColumn` (lines 223, 237)
  - Hover styling on backlog column header (lines 302-315)
  - Conditional rendering: `BacklogTableView` when expanded vs `DroppableColumn` columns (lines 1196-1231)

### Key Implementation Details

1. **Snackbar hook**: This codebase uses a custom `useSnackbar` hook from `../../hooks/useSnackbar`, NOT `notistack`. The API is `snackbar.error("message")` instead of `enqueueSnackbar("message", { variant: "error" })`.

2. **Filter state lives in BacklogTableView**: Simplified from original design - filter state (`search`, `priorityFilter`) is managed within `BacklogTableView` rather than lifted to `SpecTaskKanbanBoard`. This keeps the component self-contained.

3. **Prompt update field**: Use `description` field in the update request, not `original_prompt`. The API accepts `description` for updating the task's prompt text.

4. **Import already exists**: The `BacklogTableView` import was already added to `SpecTaskKanbanBoard.tsx` (line 68) - no need to add it.

### Build/Test Commands
```bash
cd frontend && yarn test && yarn build
```