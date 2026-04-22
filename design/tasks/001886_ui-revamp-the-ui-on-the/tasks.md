# Implementation Tasks

## Backlog phase: Replace toggle with text link

- [ ] In `SpecTaskActionButtons.tsx`, remove the `skipSpecToggle` variable (the `Switch` + `FormControlLabel` block, lines 279-305)
- [ ] Remove the `skipSpecToggle` usage from both the inline return (line 310) and the stacked return (lines 360-362)
- [ ] Make the primary button always show "Start Planning" (yellow) — remove the `just_do_it_mode` ternary for label and color (lines 272-277 and line 313/340)
- [ ] Add a `Typography` text link below the primary button in the stacked variant: "or skip to implementation" — styled as a subtle, clickable caption
- [ ] Wire the text link's `onClick` to: set `just_do_it_mode: true` via `updateSpecTaskMutation`, then call `onStartPlanning()`
- [ ] Keep the error/retry label logic: "Retry Planning" on button, "or retry as implementation" on the link when `task.metadata?.error` exists

## Spec generation phase: Remove skip button

- [ ] Delete the `if (task.status === "spec_generation")` block (lines 367-406) so no UI is rendered for tasks in this phase

## Testing

- [ ] Run `cd frontend && yarn build` to verify no build errors
- [ ] Verify in browser: backlog card shows "Start Planning" button with "or skip to implementation" link beneath
- [ ] Verify in browser: clicking "or skip to implementation" starts the task directly in implementation mode
- [ ] Verify in browser: tasks in the planning column no longer show a "Skip Planning" button
- [ ] Verify in browser: error state shows "Retry Planning" / "or retry as implementation"
