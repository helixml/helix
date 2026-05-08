# Requirements

## User Story

As a user on the spectask details page with one or more queued (non-interrupt) interactions, I want to press **Enter** on an empty chat text field to immediately send the most recently-added queued interaction — without having to find and click its lightning-bolt button.

## Acceptance Criteria

1. **Empty Enter triggers interrupt-promotion.** Pressing `Enter` (no Shift, no Ctrl/Cmd) in `RobustPromptInput`'s textarea while the draft is empty (whitespace-only also counts as empty) AND there are no pending attachments MUST locate the queued message described in (2) and flip its `interrupt` flag from `false` → `true`. This is functionally identical to the user clicking the `Zap` toggle on that queue item.

2. **"Most recent queued interaction" is well-defined.** The target is the queue entry with the highest `timestamp` among entries that satisfy ALL of:
   - `status === 'pending'` (not currently `sending`, not `failed`, not `sent`)
   - `interrupt === false` (already-interrupt entries are skipped — they're already configured to fire)
   - `deleted !== true`
   - `id !== sendingId` (don't touch the entry currently being dispatched)
   - `id !== editingId` (don't touch an entry the user is editing)

3. **Backend dispatch is unchanged.** Flipping `interrupt: true` + `syncedToBackend: false` is the existing mechanism that causes the prompt-history sync loop to push the change to the backend, which then sends the message immediately. We MUST reuse that path — no new API endpoint, no separate immediate-send call.

4. **No-op when there is no eligible target.** If no entry matches the criteria in (2) — empty queue, only interrupt-mode entries, or the only candidates are currently sending/being edited — Enter MUST do nothing (no error toast, no console noise, no draft mutation). This matches today's silent-no-op behavior on empty Enter.

5. **Existing Enter behavior is preserved when the field is non-empty.** With text or attachments present, `Enter` MUST continue to enqueue a new message (queue mode) and `Ctrl+Enter`/`Cmd+Enter` MUST continue to enqueue in interrupt mode. Shift+Enter still inserts a newline.

6. **Modifier-keys on empty Enter.**
   - `Ctrl+Enter` / `Cmd+Enter` on an empty field: same behavior as plain Enter (promote most-recent queued → interrupt). No reason to differentiate; the user is asking to "fire now" either way.
   - `Shift+Enter` on an empty field: inserts a newline (current behavior, unchanged).

7. **Disabled / offline states.** If the input is `disabled`, do nothing. If the user is offline, the flag-flip still happens locally and the existing offline-sync logic will dispatch on reconnect — no special-case needed.

## Out of Scope

- Visual hint / tooltip on the textarea telling the user "press Enter to fire most recent queued message". Possibly a nice follow-up but not required here.
- Changing the queue sort order or the lightning-icon UI.
- Keyboard shortcut to promote a *specific* (non-most-recent) queue entry.
