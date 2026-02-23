# Implementation Tasks

## Part 1: Auto-focus Input on Dialog Open

- [ ] In `NewSpecTaskForm.tsx`, add `useEffect` to focus `taskPromptRef` on component mount
- [ ] Use `setTimeout(..., 0)` to ensure focus happens after render

## Part 2: Focus Start Planning Button on New Task

- [ ] In `TaskCard.tsx`, add `focusStartPlanning?: boolean` to `TaskCardProps` interface
- [ ] In `TaskCard.tsx`, create `startPlanningButtonRef` using `useRef<HTMLButtonElement>(null)`
- [ ] In `TaskCard.tsx`, add `useEffect` that calls `startPlanningButtonRef.current?.focus()` when `focusStartPlanning` is true
- [ ] In `SpecTaskActionButtons.tsx`, add optional `startPlanningButtonRef?: React.RefObject<HTMLButtonElement>` prop
- [ ] In `SpecTaskActionButtons.tsx`, attach ref to the Start Planning `<Button>` element
- [ ] In `TaskCard.tsx`, pass `startPlanningButtonRef` to `SpecTaskActionButtons` component

## Testing

- [ ] Verify: Press Enter on kanban board → create dialog opens with textarea focused
- [ ] Verify: Create task → Start Planning button on new task has focus
- [ ] Verify: Press Enter after focus → planning starts for new task