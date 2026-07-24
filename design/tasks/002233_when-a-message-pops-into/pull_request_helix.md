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
- Verified end-to-end in the inner Helix on a real spec-task chat (Vite dev
  serving the changed source):
  - **Auto-scroll ON:** composer grown → transcript viewport shrank 500→350px
    with `scrollHeight` unchanged; `repinToBottom` kept the tail pinned
    (distFromBottom = 0) where the old `scrollToBottom` would have occluded 150px.
  - **Auto-scroll OFF:** a paused user (scrolled up 400px) is **not** yanked to
    the bottom when the composer grows; the lock stays OFF.
  - The sticky-scroll lock is never flipped by sending — pref unchanged across
    every case.
  - Same result on the mobile Chat view mount site.

## Screenshots

Auto-scroll ON — large prompt typed, transcript tail stays pinned above the grown composer:

![ON — tail pinned](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002233_when-a-message-pops-into/screenshots/02-large-prompt-tail-pinned.png)

Auto-scroll OFF — paused user is not yanked to the bottom when the composer grows:

![OFF — not yanked](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002233_when-a-message-pops-into/screenshots/03-autoscroll-off-not-yanked.png)

Mobile Chat view — same pinned-to-bottom behavior:

![Mobile — tail pinned](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002233_when-a-message-pops-into/screenshots/04-mobile-tail-pinned.png)
