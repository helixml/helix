# Implementation Tasks

## Part 1: Auto-focus Input on Dialog Open

- [x] In `NewSpecTaskForm.tsx`, add `useEffect` to focus `taskPromptRef` on component mount
- [x] Use `setTimeout(..., 0)` to ensure focus happens after render

## Part 2: Focus Start Planning Button on New Task

- [x] In `TaskCard.tsx`, add `focusStartPlanning?: boolean` to `TaskCardProps` interface
- [x] In `TaskCard.tsx`, create `startPlanningButtonRef` using `useRef<HTMLButtonElement>(null)`
- [x] In `TaskCard.tsx`, add `useEffect` that calls `startPlanningButtonRef.current?.focus()` when `focusStartPlanning` is true
- [x] In `SpecTaskActionButtons.tsx`, add optional `startPlanningButtonRef?: React.RefObject<HTMLButtonElement>` prop
- [x] In `SpecTaskActionButtons.tsx`, attach ref to the Start Planning `<Button>` element
- [x] In `TaskCard.tsx`, pass `startPlanningButtonRef` to `SpecTaskActionButtons` component

## Testing

- [ ] Verify: Press Enter on kanban board → create dialog opens with textarea focused
- [ ] Verify: Create task → Start Planning button on new task has focus
- [ ] Verify: Press Enter after focus → planning starts for new task