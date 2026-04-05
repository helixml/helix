# Implementation Tasks

## Bug 1 — Deleted prompts reappear after refresh

- [x] Add `deleted?: boolean` field to the `PromptHistoryEntry` type in `usePromptHistory.ts`
- [x] Update `removeFromQueue` to set `deleted: true` on the entry (instead of filtering it out), then save to localStorage; filter deleted entries from `pendingPrompts` / `failedPrompts` derived values
- [x] Update `mergeWithBackend` to skip re-importing entries whose ID exists locally with `deleted: true`
- [x] Update `fetchBackendHistory` (initial sync on load) to likewise skip locally-tombstoned IDs (mergeWithBackend handles this)
- [x] Update `handleRemoveFromQueue` in `RobustPromptInput.tsx` to mark as deleted first (instant UI), then fire backend DELETE (best effort)
- [x] Add tombstone cleanup: after backend DELETE succeeds, fully remove the entry from localStorage (or on next sync that doesn't return the deleted ID)
- [x] Verify: frontend builds successfully

## Bug 2 — Edit mode doesn't pause sending

- [x] Update `handleStartEdit` in `RobustPromptInput.tsx` to check entry status — if `sending`, block opening edit mode
- [x] Capture `originalContent` and `interruptMode` in edit state alongside `editingContent` (needed for cancel)
- [x] In `handleStartEdit`, after status check passes: call `apiClient.v1PromptHistoryDelete(entryId)` to remove the entry from the backend queue before opening edit UI (prevents the backend from sending it)
- [x] Update `handleSaveEdit`: tombstone old entry, call `saveToHistory(editingContent, ...)` to re-queue as a new pending entry with the edited content
- [x] Update `handleCancelEdit`: tombstone old entry, call `saveToHistory(originalContent, ...)` to re-queue the original content unchanged (so cancel truly restores the prompt)
- [x] Remove `onBlur={handleSaveEdit}` from edit textarea (conflicted with Cancel button — blur fired before click)
- [x] Verify: frontend builds successfully
