# Implementation Tasks

- [ ] In `usePromptHistory.ts`, remove the 24-hour TTL check from `loadDraft` (lines 138-152) and remove the `timestamp` field from the draft object saved in `saveDraft` (lines 154-165)
- [ ] In `NewSpecTaskForm.tsx`, define `DRAFT_KEY = \`helix_new_spectask_draft_${projectId}\``
- [ ] Change `taskPrompt` useState initializer to load the draft from localStorage (parse JSON, read `content`, fall back to `""` on any error)
- [ ] Add a `draftTimer` useRef and update the TextField's `onChange` handler to call debounced `localStorage.setItem` (300ms, no timestamp) — replacing the direct `setTaskPrompt` call with a wrapper
- [ ] Add a `useEffect` cleanup that clears the debounce timer on unmount to prevent post-unmount writes
- [ ] After successful task creation (inside the `onTaskCreated` flow), call `localStorage.removeItem(DRAFT_KEY)` to clear the draft
- [ ] Inside `resetForm()`, call `localStorage.removeItem(DRAFT_KEY)` so an explicit cancel also clears the draft
- [ ] Manual test: type text → close panel → reopen → text is restored; submit task → reopen → form is empty; cancel → reopen → form is empty
