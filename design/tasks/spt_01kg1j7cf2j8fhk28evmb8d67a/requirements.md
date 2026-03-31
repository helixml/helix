# Requirements: Persist New SpecTask Draft in localStorage

## Problem

When a user starts typing in the "new spectask" form and then navigates away (closes the panel, switches pages, goes offline), their text is gone when they return. The form resets to empty on every open.

## User Stories

**US1** — As a user writing a long spectask description, when I accidentally close the panel or lose my connection, I want my draft to be restored when I reopen the new spectask form, so I don't have to retype everything.

**US2** — As a user who submits a spectask successfully, I want the form to clear normally, so old drafts don't bleed into new tasks.

**US3** — As a user who explicitly cancels without typing much (≤ a few characters), I don't mind if the tiny draft is cleared — but for meaningful text (say > 10 characters) it should be restored.

## Acceptance Criteria

1. When the user types into the "Describe what you want to get done" textarea, the content is saved to localStorage within ~300 ms (debounced, same pattern as `usePromptHistory`).
2. When the `NewSpecTaskForm` mounts, it loads the draft from localStorage and populates `taskPrompt` if a valid draft exists.
3. After a task is **successfully submitted**, the draft is deleted from localStorage.
4. Draft has a 24-hour TTL — expired drafts are ignored on load (same as `usePromptHistory`).
5. Draft is scoped per project: key = `helix_new_spectask_draft_{projectId}` so switching projects doesn't cross-contaminate.
6. The existing `resetForm()` called on cancel does **not** need to clear the draft (let TTL handle it, or optionally clear on explicit cancel — see design decision).
