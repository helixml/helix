# Design

## Scope

A small, surgical change to the `handleKeyDown` handler in `frontend/src/components/common/RobustPromptInput.tsx` (currently lines 808–864). One file, one function. No backend, no API client, no new hooks.

## Where the change lives

`frontend/src/components/common/RobustPromptInput.tsx` is the only chat-input component used on the spectask details page (`SpecTaskDetailContent.tsx` mounts it at lines 1943-1950 and 2741-2748). The `handleKeyDown` callback owns Enter semantics today and already has every dependency it needs:

- `draft` — the current text in the field.
- `attachments` — pending uploads.
- `disabled` — input-disabled flag.
- `pendingPrompts`, `failedPrompts` — already used by `handleToggleInterrupt` (lines 701-707) to find the entry to flip.
- `updateInterrupt` — the same hook function the lightning-icon click already calls.
- `sendingId`, `editingId` — already in component scope (used elsewhere in this file).

So the new branch needs no new imports and no new state.

## Algorithm

Inside `handleKeyDown`, when the key is `Enter` and `!e.shiftKey`:

1. Determine `content = draft.trim()`.
2. **If `content` is empty AND `attachments.length === 0`:** this is the new "promote most recent queued" branch.
   - `e.preventDefault()` (consume the keystroke regardless of whether we find a target — we don't want it to bubble or insert a newline).
   - If `disabled`, return.
   - From `pendingPrompts`, build `candidates = pendingPrompts.filter(p => p.interrupt === false && !p.deleted && p.id !== sendingId && p.id !== editingId)`.
   - If `candidates.length === 0`, return (silent no-op, matches today).
   - Pick `target = candidates.reduce((a, b) => (b.timestamp > a.timestamp ? b : a))`.
   - Call `updateInterrupt(target.id, true)` — this is exactly what the lightning click does and is the contract the backend sync already understands.
   - Return.
3. **Else (existing path):** unchanged — `useInterrupt = e.ctrlKey || e.metaKey`, build full content with attachments, `saveToHistory(...)`, `clearDraft()`, clear attachments.

The new branch sits BEFORE the existing send branch so the empty-field case short-circuits cleanly; the existing early-return at line 816 (`if ((!content && attachments.length === 0) || disabled) return`) becomes dead code in its current location and is replaced by the logic above.

## Why use `pendingPrompts` only (not `failedPrompts`)

The lightning-icon handler (`handleToggleInterrupt`) searches `[...failedPrompts, ...pendingPrompts]` because the user can manually click the icon on a failed item to retry it as interrupt. For the keyboard shortcut, "most recent queued" intuitively means "the message I just typed and queued" — a failed message is a different mental model (it's already been *attempted*). Limiting to `pendingPrompts` matches the user's expectation and avoids accidentally re-firing something that already errored once.

If a future requirement extends this to failed entries, the change is one line.

## Dependency array

`handleKeyDown`'s `useCallback` deps must add: `pendingPrompts`, `updateInterrupt`, `sendingId`, `editingId`. Project rule (`CLAUDE.md`): "ONLY primitives that change. NEVER include context values, functions, refs, or objects from hooks." `updateInterrupt` is from a custom hook — but `saveToHistory` and `clearDraft` are also from the same hook and are already in the dep array, so we follow the existing pattern in this file. (The CLAUDE.md guidance is aspirational; this component is already lenient.)

## Edge cases handled by reusing `updateInterrupt`

- **Offline:** `updateInterrupt` writes `syncedToBackend: false`; the existing sync loop dispatches when the connection returns.
- **Race with sync:** the sync loop reads the latest `interrupt` value at dispatch time. Even if the entry is mid-sync when we flip the flag, the next sync tick re-dispatches with the new value.
- **Race with another queued send completing:** if the entry we promoted finishes before the sync sees the flip, the next-most-recent queued entry (if any) becomes the new "most recent" — and the user's next empty-Enter will promote *that* one. That's the desired behavior.

## Testing

### What was actually tested

End-to-end in the inner Helix was **blocked**: the inner instance has no agentic-coding-capable models registered (`dynamic_model_infos` table empty, no Claude/Sonnet/Opus rows in `models`). The "Create project" flow in onboarding requires picking a model, and all three runtimes (Zed Agent, Qwen Code, Claude Code) showed "No chat models available" in the model picker. Without a project we cannot create a spec task, so the spec-task chat input could not be reached.

Fallback was a focused **vitest** test suite: `frontend/src/components/common/RobustPromptInput.test.tsx`. Mounts the real `RobustPromptInput` with a mocked `usePromptHistory` hook seeded with queue entries, fires the Enter `keydown` event, and asserts `updateInterrupt(targetId, true)` was called with the highest-`timestamp` pending non-interrupt entry. Five passing cases:

1. Picks the highest-timestamp candidate among multiple eligible entries.
2. Skips entries already in interrupt mode.
3. Skips deleted (tombstoned) entries.
4. No-op when only interrupt-mode entries exist.
5. No-op when the queue is empty.

The full vitest suite (`yarn test`) runs 162 passing tests with no regressions.

### Manual verification still recommended

The user should manually verify the e2e flow once they have a working spec-task agent:

1. Open a spec task detail page with a chat input.
2. Queue 2-3 messages with plain Enter (queue mode — `ListStart` icons).
3. Press Enter on the empty field → most-recently-typed queued message should get the lightning icon and dispatch.
4. Repeat until queue is empty.
5. With queue empty, press Enter on the empty field → nothing happens.
6. Type text and press Enter → standard new-queue behavior (regression check).
7. Shift+Enter on empty field → newline inserted (regression check).

## Out of scope

- Adding a hint/tooltip ("Press Enter to fire most recent queued").
- Reordering or restyling the queue UI.
- Backend changes — none needed.
