# Implementation Tasks

## Bug 1 — Deleted prompts reappear after refresh

- [x] Add `deleted?: boolean` field to the `PromptHistoryEntry` type in `usePromptHistory.ts`
- [x] Update `removeFromQueue` to set `deleted: true` on the entry (instead of filtering it out), then save to localStorage; filter deleted entries from `pendingPrompts` / `failedPrompts` derived values
- [x] Update `mergeWithBackend` to skip re-importing entries whose ID exists locally with `deleted: true`
- [x] Update `fetchBackendHistory` (initial sync on load) to likewise skip locally-tombstoned IDs (mergeWithBackend handles this)
- [x] Update `handleRemoveFromQueue` in `RobustPromptInput.tsx` to mark as deleted first (instant UI), then fire backend DELETE (best effort)
- [x] Add tombstone cleanup: after backend DELETE succeeds, fully remove the entry from localStorage (or on next sync that doesn't return the deleted ID)
- [ ] Verify: delete a queued prompt, refresh immediately, confirm it does not reappear

## Bug 2 — Edit mode doesn't pause sending

- [~] Update `handleStartEdit` in `RobustPromptInput.tsx` to check entry status — if `sending`, show a snackbar/toast ("Already being sent — cannot edit") and abort opening edit mode
- [~] Capture `originalContent` in edit state alongside `editingContent` (needed for cancel)
- [~] In `handleStartEdit`, after status check passes: call `handleRemoveFromQueue(entryId)` to remove the entry from the backend queue before opening edit UI (prevents the backend from sending it)
- [~] Update `handleSaveEdit`: instead of calling `updateContent`, call `saveToHistory(editingContent, ...)` to re-queue as a new pending entry with the edited content
- [~] Update `handleCancelEdit`: call `saveToHistory(originalContent, ...)` to re-queue the original content unchanged (so cancel truly restores the prompt)
- [ ] Verify: start editing a queued prompt, wait 2+ seconds (long enough for backend polling), confirm the prompt is not sent; save edit and confirm new content is queued; cancel and confirm original content is queued
- [ ] Verify: attempt to edit a prompt that transitions to `sending` just as edit is clicked — confirm graceful error, not silent data loss
