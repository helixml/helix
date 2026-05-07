# Implementation Tasks

- [~] In `frontend/src/components/common/RobustPromptInput.tsx` `handleKeyDown` (around lines 808-864), add an "empty Enter promotes most-recent queued to interrupt" branch that runs before the existing send branch when `draft.trim()` is empty and `attachments.length === 0`.
- [~] In that branch, filter `pendingPrompts` to entries where `interrupt === false`, `!deleted`, `id !== sendingId`, and `id !== editingId`; pick the entry with the highest `timestamp`; call `updateInterrupt(target.id, true)`. If no candidate exists, silently return.
- [~] Update the `useCallback` dependency array of `handleKeyDown` to include `pendingPrompts`, `updateInterrupt`, `sendingId`, and `editingId`.
- [ ] Verify the existing send behavior is unchanged when the field has text or attachments (Enter, Ctrl+Enter, Cmd+Enter, Shift+Enter all behave as today).
- [ ] Run `cd frontend && yarn build` to confirm TypeScript and the build pass.
- [ ] End-to-end test in the inner Helix at `http://localhost:8080`: register/login, open a spectask, queue 2-3 plain-Enter messages, then press Enter on the empty field and confirm the most-recently-typed queued message switches to interrupt mode and is dispatched. Repeat until queue is empty; confirm subsequent empty-Enter is a no-op.
- [ ] Open a PR against `helixml/helix` referencing this task; include a short note in the PR body that points at the design doc path.
