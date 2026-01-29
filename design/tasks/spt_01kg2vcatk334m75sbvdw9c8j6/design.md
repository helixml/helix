# Design: Backlog Prompt Tooltip & Table View

## Architecture Overview

This feature adds two UI enhancements to the Kanban board in `SpecTaskKanbanBoard.tsx` and `TaskCard.tsx`:

1. **Tooltip on hover** - Shows full prompt when hovering over task cards
2. **Expandable table view** - Clicking backlog header toggles table view with drag-to-reorder

## Key Patterns Found in Codebase

- **Tooltips**: MUI `Tooltip` component is used throughout (see `OAuthConnections.tsx`, `AgentSandboxes.tsx`)
- **Drag-and-drop**: `@dnd-kit` is already used in `RobustPromptInput.tsx` for sortable lists
- **Task data**: `original_prompt` field contains the full prompt text (see `TypesSpecTask` in `api.ts`)
- **Column headers**: Rendered in `DroppableColumn` component within `SpecTaskKanbanBoard.tsx`

## Implementation Approach

### Feature 1: Prompt Tooltip on Task Cards

**Location**: `TaskCard.tsx`

Wrap the Card component with MUI `Tooltip`:
- Use `task.original_prompt` or fall back to `task.description` or `task.name`
- Set `enterDelay={300}` to avoid accidental triggers
- Add `sx={{ maxHeight: 300, overflow: 'auto' }}` to tooltip content for long prompts

### Feature 2: Backlog Table View

**Location**: `SpecTaskKanbanBoard.tsx`

Add state to track expanded view:
```
const [backlogExpanded, setBacklogExpanded] = useState(false)
```

When expanded:
- Render a `Box` with `width: 100%` instead of the normal column
- Use MUI `Table` with columns: Name, Prompt, Priority, Created
- Wrap table body in `DndContext` + `SortableContext` from `@dnd-kit`
- Each row is a `SortableTableRow` component

**Reordering persistence** (optional, can defer):
- Add `sort_order: number` field to `SpecTask` type
- Add API endpoint `PATCH /api/v1/spec-tasks/{id}/reorder` 
- For MVP, can use optimistic UI with localStorage fallback

## Components to Modify

| File | Change |
|------|--------|
| `TaskCard.tsx` | Add Tooltip wrapper around Card |
| `SpecTaskKanbanBoard.tsx` | Add backlog expanded state, BacklogTableView component |
| `api/pkg/types/simple_spec_task.go` | Add `SortOrder int` field (optional) |
| `api/pkg/server/spec_driven_task_handlers.go` | Add reorder endpoint (optional) |

## UI Mockup (ASCII)

**Collapsed (current)**:
```
┌─────────────┐ ┌─────────────┐
│ Backlog (3) │ │ Planning    │
├─────────────┤ ├─────────────┤
│ Task 1      │ │ Task A      │
│ Task 2      │ │             │
│ Task 3      │ │             │
└─────────────┘ └─────────────┘
```

**Expanded (new)**:
```
┌──────────────────────────────────────────────────────────┐
│ ▼ Backlog (3)                                            │
├──────────────────────────────────────────────────────────┤
│ ☰ Task 1    │ "Create auth system..."    │ High   │ 1/15 │
│ ☰ Task 2    │ "Add dark mode toggle..."  │ Medium │ 1/14 │
│ ☰ Task 3    │ "Fix login bug..."         │ Low    │ 1/13 │
└──────────────────────────────────────────────────────────┘
```

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Long prompts in tooltip hard to read | Max height with scroll, monospace font |
| Reorder not persisting | Start with localStorage, add backend later |
| Table view breaks layout on small screens | Only show expand option on larger breakpoints |