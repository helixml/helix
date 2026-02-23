# Design: Focus Behavior After Task Creation

## Overview

Implement automatic focus management for two scenarios:
1. Focus the task input field when the create dialog opens via Enter key
2. Focus the "Start Planning" button on a newly created task card

## Current State

### Existing Infrastructure
- `focusTaskId` state exists in `SpecTasksPage.tsx` - tracks newly created task ID
- `focusStartPlanning` prop is passed to `TaskCard` via `SpecTaskKanbanBoard`
- However, `TaskCard` does NOT actually use `focusStartPlanning` to focus anything
- `NewSpecTaskForm` has a `taskPromptRef` for the textarea but doesn't auto-focus it

### Key Files
- `helix/frontend/src/pages/SpecTasksPage.tsx` - main page with `handleTaskCreated`
- `helix/frontend/src/components/tasks/NewSpecTaskForm.tsx` - create task form
- `helix/frontend/src/components/tasks/TaskCard.tsx` - task card with Start Planning button
- `helix/frontend/src/components/tasks/SpecTaskKanbanBoard.tsx` - board with Enter key handler

## Solution Design

### Part 1: Focus Input on Dialog Open

In `NewSpecTaskForm.tsx`:
- Add `useEffect` that focuses `taskPromptRef` when component mounts
- Use `setTimeout` with 0ms delay to ensure DOM is ready

```tsx
useEffect(() => {
  setTimeout(() => {
    taskPromptRef.current?.focus()
  }, 0)
}, [])
```

### Part 2: Focus Start Planning Button

In `TaskCard.tsx`:
1. Add `focusStartPlanning` to `TaskCardProps` interface
2. Create a `buttonRef` for the Start Planning button
3. Add `useEffect` that focuses the button when `focusStartPlanning` is true
4. Attach ref to the Start Planning button in `SpecTaskActionButtons`

Since `SpecTaskActionButtons` is a separate component, we need to either:
- **Option A**: Pass a ref down through props (chosen approach - simpler)
- **Option B**: Use `forwardRef` on `SpecTaskActionButtons`

The ref will be passed via a new `startPlanningButtonRef` prop on `SpecTaskActionButtons`.

## Component Changes

| File | Change |
|------|--------|
| `NewSpecTaskForm.tsx` | Add auto-focus effect on mount |
| `TaskCard.tsx` | Add `focusStartPlanning` prop handling with useEffect + ref |
| `SpecTaskActionButtons.tsx` | Add optional `startPlanningButtonRef` prop |

## Testing

1. Press Enter on kanban board → verify textarea is focused
2. Create a task → verify Start Planning button has focus
3. Press Enter after task creation → verify planning starts