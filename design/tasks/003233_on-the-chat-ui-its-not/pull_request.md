# Clarify the selected task in the chat sidebar

## Summary
Add a subtle inset border to the chat sidebar's existing selected-row background so the current task is easier to identify without disrupting the established visual hierarchy. Mark the selected row with `aria-current` and add focused regression coverage for the active state.

## Testing
`git diff --check` passes. Added a focused `ProjectChatItemRow` test for the selected background, inset border, and `aria-current` state. The test could not be executed in the task sandbox because frontend dependencies were absent and `yarn install --frozen-lockfile` repeatedly failed during network fetches.
