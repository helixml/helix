# Re-pin spec-task chat to the bottom when the composer grows

## Summary

In the Spec-Task detail chat panel, queuing/sending a message grows the composer
(the queue panel expands and the textarea auto-resizes). Because the composer is
a `flexShrink: 0` sibling below the transcript, its growth shrinks the
transcript's scroll **viewport** while the transcript's content `scrollHeight` is
unchanged — so the tail of the conversation slides below the fold and is
occluded. With larger prompts the disruption was big enough that users
reflexively scrolled up to reorient, crossing the 100px unlock threshold and
inadvertently **disengaging sticky-scroll**.

The parent already tried to compensate via `onHeightChange → scrollToBottom()`,
but `scrollToBottom` short-circuits when `scrollHeight` hasn't changed since the
last write (`EmbeddedSessionView.tsx`). Since only the viewport shrank, that
guard fired and no scroll was written.

## Fix

Add `EmbeddedSessionViewHandle.repinToBottom()` — a dedicated "re-pin on layout
change" primitive that:

- **Bypasses** the `scrollHeight`-unchanged short-circuit (a viewport shrink
  needs a scroll write even though content height didn't change).
- **Respects** the auto-scroll preference — it's a no-op when auto-scroll is OFF,
  so a user who deliberately paused is left where they are.
- **Never** calls `setAutoScroll`/`triggerUnlock`, so simply sending a message can
  never flip the sticky-scroll lock.
- Pre-records `lastScrolledHeightRef`/`lastScrollTopRef` and clears `hasNewBelow`
  exactly like `scrollToBottom`, so the resulting `onScroll` sees no delta.

`scrollToBottom` (and its short-circuit) is left intact for the poll/WS-update
path, so keepalives still do no redundant scroll work.

## Changes

- `EmbeddedSessionView.tsx`: add `repinToBottom()` to `EmbeddedSessionViewHandle`,
  implement it, and expose it via `useImperativeHandle`.
- `SpecTaskDetailContent.tsx`: both composer mount sites (desktop + mobile) call
  `repinToBottom()` from `onHeightChange` instead of `scrollToBottom()`.
- `HelixOrgBotDetail.tsx` and `HelixOrgChatPanel.tsx`: the two other surfaces with
  the identical `onHeightChange → scrollToBottom()` wiring and the same occlusion
  bug are switched to `repinToBottom()` for consistency.

## Testing

- `cd frontend && yarn build` passes (`✓ built`).
- Manual end-to-end verification in the inner Helix chat panel — see the task's
  design.md testing checklist (AC-1 … AC-7).
