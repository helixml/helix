# Persist new spectask draft in localStorage

## Summary

When a user starts typing in the "New SpecTask" form and then closes the panel (X button), navigates away, or loses their connection, their text is now saved to localStorage and restored when they reopen the form.

Also removes the 24-hour TTL from the existing prompt draft persistence in `usePromptHistory` — drafts now persist indefinitely until explicitly cleared.

## Changes

- `NewSpecTaskForm.tsx`: Load draft from `helix_new_spectask_draft_{projectId}` on mount; debounce-save on every keystroke (300ms); clear on successful submit or explicit cancel
- `usePromptHistory.ts`: Remove TTL check from `loadDraft` and `timestamp` field from `saveDraft` / `PromptDraft` type
