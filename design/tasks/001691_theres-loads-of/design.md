# Design: Prompt Queue Flakiness Fixes

## Codebase context

- Hook: `frontend/src/hooks/usePromptHistory.ts` — all queue state, localStorage, backend sync
- UI: `frontend/src/components/common/RobustPromptInput.tsx` — queue display, edit mode, delete handler
- Backend handler: `api/pkg/server/prompt_history_handlers.go` — HTTP endpoints, queue processing
- Backend store: `api/pkg/store/store_prompt_history.go` — DB queries (soft-delete pattern, `deleted_at IS NULL` guards)
- Polling: frontend polls every 2 s while pending/failed messages exist (`useEffect` at line ~385 in `usePromptHistory.ts`)

---

## Fix 1 — Deletion persistence

### Problem
`mergeWithBackend` is a pure union merge: it adds backend entries that don't exist locally, but never removes entries that were deleted locally. So a deleted entry re-appears if the browser reloads before the backend DELETE completes.

### Solution: local tombstone (deleted flag)

Add a `deleted: boolean` field to the local `PromptHistoryEntry` type (localStorage only — not synced to backend).

**Delete flow (new):**
1. Mark entry as `deleted: true` in local state + save to localStorage. Entry is hidden from the queue UI immediately.
2. Fire `v1PromptHistoryDelete` in the background (best effort).

**`mergeWithBackend` change:**
- When adding `newEntries` from the backend, skip any whose ID is already present in localStorage with `deleted: true`.
- Also, when the initial sync loads the full list from backend, filter against the local tombstone set.

**Tombstone cleanup:** Remove tombstoned entries from localStorage after they have been confirmed deleted on the backend (check response from DELETE, or on next successful sync that doesn't return the entry).

This approach:
- Gives instant UI response (no latency)
- Survives page refresh (tombstone persists in localStorage)
- Handles offline gracefully (tombstone prevents re-import until backend catches up)

**Files to change:**
- `frontend/src/hooks/usePromptHistory.ts`: add `deleted` to `PromptHistoryEntry`, update `removeFromQueue`, update `mergeWithBackend`, update `loadHistory` / `saveHistory`
- `frontend/src/components/common/RobustPromptInput.tsx`: `handleRemoveFromQueue` — change to mark deleted first, then fire backend call

---

## Fix 2 — Edit mode actually pauses sending

### Problem
`editingId` is ephemeral React state; the backend processes all `pending` entries on its own schedule. There is no backend concept of "this entry is being edited."

### Solution: optimistic status change to "editing" / "on-hold"

The simplest safe approach that requires no backend changes:

**When entering edit mode:**
1. Check the entry's current status. If it is already `sending`, show an error toast ("This prompt is already being sent — cannot edit") and do not open the edit UI.
2. If it is `pending`, immediately change the entry's status locally to `"editing"` (a new client-only transient status) and sync this to the backend as a content-only update (the backend doesn't need to understand `"editing"`, but the sync call updates the content if needed and keeps the entry alive).
   - Better: use the existing `updateContent` path but also mark the entry as paused by temporarily **removing it from the backend queue** (call `v1PromptHistoryDelete`), cache its content locally, and re-create it on save.

**Preferred approach — delete-then-recreate:**
When the user clicks edit on a `pending` entry:
1. Store the entry content in `editingContent` state (existing).
2. Call `handleRemoveFromQueue(entryId)` using the new tombstone-based delete — the entry is removed from the backend queue and cannot be sent.
3. Show the edit UI with the cached content.
4. On **save**: call `saveToHistory(editedContent, ...)` to re-queue the edited prompt as a new pending entry. The old entry stays deleted.
5. On **cancel**: call `saveToHistory(originalContent, ...)` to re-queue the original content unchanged.

This guarantees the backend has nothing to send while editing. The UX is unchanged from the user's perspective (prompt stays in queue, content updates on save).

**Edge case:** if the entry was in `sending` status when edit was clicked (race), step 1 prevents entry and shows a message.

**Files to change:**
- `frontend/src/components/common/RobustPromptInput.tsx`:
  - `handleStartEdit`: check status, block if `sending`
  - `handleSaveEdit`: delete old entry, re-queue with new content
  - `handleCancelEdit`: delete old entry, re-queue with original content
  - May need to pass `originalContent` alongside `editingContent` in edit state

---

## Patterns found in codebase

- This project uses a **localStorage-backed optimistic queue** that syncs to PostgreSQL via a periodic poll + immediate sync on new entries. The backend is the source of truth for `status` (pending/sending/sent/failed) but the frontend manages the queue items themselves.
- `mergeWithBackend` is intentionally additive (union merge) to handle offline scenarios — this must remain additive but needs awareness of local deletions.
- Backend uses `FOR UPDATE SKIP LOCKED` for atomic claiming — concurrent sends are safe; the edit mode problem is purely a frontend concern about preventing entries from being in `pending` state while editing.
- The `syncPromptHistory` endpoint returns the current backend entries — if an entry is deleted before sync returns, it won't appear in the response.

---

## Implementation Notes

### Bug 1 — Tombstone approach
- Used `deleted?: boolean` field on `PromptHistoryEntry` (localStorage only, not synced to backend).
- `removeFromQueue` now marks `deleted: true` instead of filtering. This keeps the tombstone in localStorage to block re-import.
- All merge points (`mergeWithBackend`, `syncToBackend`, poll handler) skip tombstoned entries.
- Tombstone cleanup: in `mergeWithBackend`, if a tombstoned ID is no longer in the backend response, the tombstone is removed from localStorage.
- `sessionHistory` (the hook's public `history` return) filters out `deleted` entries, so they're invisible in the UI.

### Bug 2 — Delete-from-backend-then-requeue approach
- Chose NOT to tombstone locally during edit (would hide the entry from the queue UI where the edit form renders).
- Instead: `handleStartEdit` fires `apiClient.v1PromptHistoryDelete(entry.id)` to remove from backend only. Entry stays visible locally for the edit UI.
- On save/cancel: tombstone old local entry via `removeFromQueue`, then `saveToHistory` to create a new pending entry with the (possibly edited) content.
- This guarantees the backend has nothing to send during editing.
- Removed `onBlur={handleSaveEdit}` from the edit textarea — it conflicted with the Cancel button (blur fires before click, causing save instead of cancel).

### Files modified
- `frontend/src/hooks/usePromptHistory.ts` — tombstone field, merge guards, sync guards
- `frontend/src/components/common/RobustPromptInput.tsx` — edit flow rewrite, onBlur removal

### Gotcha: onBlur vs Cancel button
The original code had `onBlur={handleSaveEdit}` on the edit textarea. When the user clicked the Cancel button, the blur event fired first (saving the edit), then the cancel click fired (but `editingId` was already null, so cancel was a no-op). This was a pre-existing bug, but became more visible with the new delete-and-requeue flow. Fixed by removing onBlur entirely — users save with Enter or the Save button.
