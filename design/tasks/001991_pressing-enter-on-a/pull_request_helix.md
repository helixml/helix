# Empty Enter on chat input promotes most-recent queued interaction to interrupt

## Summary
On the spec-task details page, pressing **Enter** in the chat textarea while the field is empty (no text, no attachments) now flips the most-recently-typed queued interaction from queue mode to interrupt mode — same effect as clicking the lightning-bolt button on that queue item, which causes it to dispatch immediately.

Previously, empty Enter was a silent no-op. The new behavior is a power-user shortcut: type a few messages, then keep tapping Enter on an empty field to fire each one in reverse-typed order, instead of mousing over to the lightning icon.

## Changes
- `frontend/src/components/common/RobustPromptInput.tsx` — `handleKeyDown` now has an explicit empty-field branch. It filters `pendingPrompts` to entries where `interrupt === false`, `!deleted`, `id !== sendingId`, `id !== editingId`, picks the highest-`timestamp` candidate, and calls `updateInterrupt(id, true)` (the same hook the lightning-icon click calls). Silently no-ops when no candidate exists.
- `frontend/src/components/common/RobustPromptInput.test.tsx` (new) — 5 vitest cases exercising the new branch.

## Behaviour preserved
Non-empty Enter, Ctrl/Cmd+Enter, Shift+Enter, and the lightning-icon click are all unchanged. The new branch returns before the existing send code path.

## Test plan
- [x] `cd frontend && yarn build` (green)
- [x] `cd frontend && yarn test` — 162/162 passing including 5 new tests for this branch
- [ ] Manual e2e in inner Helix was blocked (no Claude/agentic-coding models registered in the inner instance); user to verify on a working spec-task agent — see `design/tasks/001991_pressing-enter-on-a/design.md` for the manual checklist.

Design doc: `design/tasks/001991_pressing-enter-on-a/` in the `helix-specs` branch of this repo.
