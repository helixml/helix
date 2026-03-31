# Implementation Tasks

- [ ] In `NewSpecTaskForm.tsx`, define `DRAFT_KEY = \`helix_new_spectask_draft_${projectId}\`` and `DRAFT_TTL` constant (24h)
- [ ] Change `taskPrompt` useState initializer to load and validate draft from localStorage (parse JSON, check TTL, fall back to `""`)
- [ ] Add a `draftTimer` useRef and update the TextField's `onChange` handler to call debounced `localStorage.setItem` (300ms) — replacing the direct `setTaskPrompt` call with a wrapper
- [ ] Add a `useEffect` cleanup that clears the debounce timer on unmount to prevent post-unmount writes
- [ ] After successful task creation (inside the `onTaskCreated` flow), call `localStorage.removeItem(DRAFT_KEY)` to clear the draft
- [ ] Inside `resetForm()`, call `localStorage.removeItem(DRAFT_KEY)` so an explicit cancel also clears the draft
- [ ] Manual test: type text → close panel → reopen → text is restored; submit task → reopen → form is empty; cancel → reopen → form is empty; wait 24h (or mock timestamp) → draft is ignored
