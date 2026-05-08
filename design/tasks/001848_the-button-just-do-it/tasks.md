# Implementation Tasks

- [x] In `SpecTaskActionButtons.tsx` (lines 268-273): rename backlog button label from `"Just Do It"` to `"Start Implementation"` and error-state label from `"Retry"` to `"Retry Implementation"`
- [x] In `SpecTaskActionButtons.tsx` (backlog phase, ~lines 249-329): add a "Skip spec" toggle (Switch or Checkbox) next to the action button that calls the existing update API to flip `just_do_it_mode`
- [x] In `NewSpecTaskForm.tsx`: rename the "Just do it" checkbox label to "Skip spec" and update placeholder/helper text to describe skipping spec generation
- [x] In `SpecTaskDetailContent.tsx` (lines 1306-1316): update the edit-mode checkbox label to "Skip spec" for consistency
- [x] Verify the keyboard shortcut (Ctrl/Cmd+J) in `NewSpecTaskForm.tsx` still works and any tooltip references are updated
- [x] Manual QA: create a task with "Skip spec" toggled on, verify backlog shows "Start Implementation", click it, confirm task goes to `queued_implementation` status
- [x] Manual QA: on an existing backlog task, toggle "Skip spec" on/off from the card, verify button label updates between "Start Planning" and "Start Implementation"
