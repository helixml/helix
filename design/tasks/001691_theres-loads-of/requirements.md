# Requirements: Prompt Queue Flakiness Fixes

## Bug 1 — Deleted prompts reappear after page refresh

**User story:** As a user, when I delete a prompt from the queue and then refresh the page, the deleted prompt must not come back.

**Root cause:** `handleRemoveFromQueue` removes the entry from localStorage immediately but fires `v1PromptHistoryDelete` asynchronously. Refreshing the page before the HTTP DELETE completes causes the browser to abort the in-flight request. On reload, `fetchBackendHistory` re-imports the entry via `mergeWithBackend` (union merge — it only adds, never removes).

**Acceptance criteria:**
- Deleting a prompt and immediately refreshing never resurfaces the deleted entry.
- The queue UI removes the entry instantly (no latency regression).
- If the backend DELETE fails (offline, network error), the entry is still suppressed locally.

---

## Bug 2 — "Paused during edit" doesn't actually pause sending

**User story:** As a user, when I click to edit a queued prompt, sending is genuinely paused until I save or cancel. The prompt must not be sent while I'm editing it.

**Root cause:** `editingId` is frontend-only React state. The backend's `processPendingPromptsForIdleSessions()` runs on every sync/poll (every 2 s) and processes all `pending` entries regardless. The message gets claimed as `sending` → `sent` while the user is mid-edit, then disappears from the queue, discarding the in-progress edit.

**Acceptance criteria:**
- A prompt being edited cannot be sent or claimed by the backend until the edit is saved or cancelled.
- If a prompt transitions to `sending` while the edit UI is open, the user sees a clear warning (not a silent loss).
- Cancelling an edit restores the original prompt in the queue unchanged.
- Saving an edit updates the prompt content and re-queues it (still pending, not re-submitted).
