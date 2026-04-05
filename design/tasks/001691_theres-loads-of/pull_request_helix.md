# Fix prompt queue flakiness: deletion persistence and edit-mode pause

## Summary
Fixes two prompt queue bugs: (1) deleted prompts reappearing after page refresh, and (2) "paused during edit" not actually preventing the backend from sending the prompt.

## Changes
- Add tombstone-based deletion in `usePromptHistory.ts`: `removeFromQueue` now marks entries as `deleted: true` in localStorage instead of filtering them out, preventing backend sync from re-importing deleted entries
- Guard all merge/sync points (`mergeWithBackend`, `syncToBackend`, poll handler) against tombstoned entries
- Add tombstone cleanup: entries are removed from localStorage once the backend confirms deletion
- Rewrite edit flow in `RobustPromptInput.tsx`: when editing starts, the entry is deleted from the backend queue (via `v1PromptHistoryDelete`); on save/cancel, the old entry is tombstoned locally and a new pending entry is created with the (possibly edited) content
- Block editing entries that are already in `sending` status
- Remove `onBlur={handleSaveEdit}` from edit textarea (conflicted with Cancel button)
