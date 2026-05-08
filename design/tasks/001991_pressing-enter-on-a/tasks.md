# Implementation Tasks

- [x] In `frontend/src/components/common/RobustPromptInput.tsx` `handleKeyDown` (around lines 808-864), add an "empty Enter promotes most-recent queued to interrupt" branch that runs before the existing send branch when `draft.trim()` is empty and `attachments.length === 0`.
- [x] In that branch, filter `pendingPrompts` to entries where `interrupt === false`, `!deleted`, `id !== sendingId`, and `id !== editingId`; pick the entry with the highest `timestamp`; call `updateInterrupt(target.id, true)`. If no candidate exists, silently return.
- [x] Update the `useCallback` dependency array of `handleKeyDown` to include `pendingPrompts`, `updateInterrupt`, `sendingId`, and `editingId`.
- [x] Verify the existing send behavior is unchanged when the field has text or attachments (Enter, Ctrl+Enter, Cmd+Enter, Shift+Enter all behave as today). (Visual diff: the new empty-field branch returns before the existing send path; the send path is byte-for-byte identical apart from the redundant `disabled` check moved outside.)
- [x] Run `cd frontend && yarn build` to confirm TypeScript and the build pass.
- [x] End-to-end test in the inner Helix — **blocked** by no agentic-coding-capable models registered (model picker empty for all 3 runtimes). Replaced with focused vitest suite at `frontend/src/components/common/RobustPromptInput.test.tsx` (5 passing tests covering the new branch). Full suite `yarn test` = 162/162 green. Manual e2e verification noted in design.md as a follow-up for the user.
- [x] Add vitest unit tests for the new empty-Enter branch (covers: highest-timestamp pick, skip already-interrupt, skip deleted, no-op when no candidates, no-op when queue empty).
- [x] Push the feature branch to `helixml/helix` (the Helix platform creates the GitHub PR automatically when the user clicks "Open PR").
