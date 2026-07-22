# Design: Keep Spec-Task Chat Pinned to Bottom When the Message Queue Grows

## Components

- `frontend/src/components/session/EmbeddedSessionView.tsx` — the scrollable
  transcript; owns the auto-scroll model, `scrollToBottom`, and the
  `EmbeddedSessionViewHandle` ref API.
- `frontend/src/components/common/RobustPromptInput.tsx` — the composer; renders
  the local/backend message queue and calls `onHeightChange()` when the queue
  panel expands/collapses (`useEffect` on `queueLength`, ~50ms after the Collapse
  starts) or the textarea auto-resizes (`adjustHeight`).
- `frontend/src/components/tasks/SpecTaskDetailContent.tsx` — wires
  `onHeightChange={() => sessionViewRef.current?.scrollToBottom()}` at both the
  desktop (`onHeightChange` ~line 2033, `ref` ~2014) and mobile (`onHeightChange`
  ~line 2838, `ref` ~2810) composer mount sites.

## Root Cause

The transcript container and the composer are siblings in a flex column; the
composer has `flexShrink: 0`. When a message is queued, the composer grows, which
**shrinks the transcript's viewport `clientHeight`** while the transcript's
content `scrollHeight` is unchanged. Since `scrollTop` stays put, the bottom of
the conversation slides below the (now shorter) viewport — the tail is occluded.

The parent tries to compensate via `onHeightChange → scrollToBottom()`, but:

1. The ref exposes only the **non-force** `scrollToBottom` (`force = false`).
2. `scrollToBottom` short-circuits at `EmbeddedSessionView.tsx:204`:
   `if (!force && container.scrollHeight === lastScrolledHeightRef.current) return;`
   Because only the viewport shrank (content height unchanged), this guard fires
   and **no scroll is written** — the tail stays hidden.

The "auto-scroll disengages" symptom (US-3) is a **downstream effect**: the
bigger the prompt, the bigger the occlusion; the user reflexively wheels up to
reorient and crosses `USER_SCROLL_UNLOCK_PX` (100px), flipping the lock OFF via
`triggerUnlock()`. Keeping the transcript pinned removes the reason to scroll,
eliminating the accidental unlock. (See Open Question 1 for the case where the
lock flips with no user scroll at all.)

## Approach

Introduce a dedicated "re-pin to bottom on layout change" path that the composer
height-change handler uses, distinct from both the polling `scrollToBottom()` and
the force paths (mount/jump-pill). It must:

- **Respect the preference:** only re-pin when auto-scroll is ON (satisfies AC-3
  and AC-4 — never yank a user who paused).
- **Ignore the `scrollHeight`-unchanged guard:** a viewport shrink needs a scroll
  write even though content height did not change (satisfies AC-2). Writing
  `scrollTop = scrollHeight` when already at the bottom is idempotent and cheap,
  so bypassing the guard here is safe.
- **Never touch the lock:** it only reads `autoScrollRef` and writes `scrollTop`;
  it never calls `setAutoScroll`/`triggerUnlock` (satisfies AC-3).
- **Pre-record refs:** set `lastScrolledHeightRef`, `lastScrollTopRef`, and clear
  `hasNewBelow` exactly as `scrollToBottom` does, so the resulting `onScroll`
  sees no positive delta and does not spuriously re-enable/alter state (AC-7).

### Concrete change

Add a method to `EmbeddedSessionViewHandle`, e.g. `repinToBottom()`, exposed via
`useImperativeHandle`, implemented as a small callback:

```
const repinToBottom = useCallback(() => {
  const container = containerRef.current;
  if (!container) return;
  if (!autoScrollRef.current) return;        // respect the pref (AC-4)
  container.scrollTop = container.scrollHeight; // bypass scrollHeight guard (AC-2)
  lastScrolledHeightRef.current = container.scrollHeight;
  lastScrollTopRef.current = container.scrollHeight;
  setHasNewBelow(false);
}, []);
```

Update the parent handlers in `SpecTaskDetailContent.tsx` (both mount sites) to
call `sessionViewRef.current?.repinToBottom()` from `onHeightChange` instead of
`scrollToBottom()`.

Leave `scrollToBottom` (and its `scrollHeight` short-circuit) intact for the
poll/WS-update path so AC-7's "no redundant scroll work" guarantee is preserved.

### Why not just call `scrollToBottom(true)`?

`force = true` also bypasses the auto-scroll preference (it exists for
mount/session-change/jump-pill, which must scroll regardless). Using it from
`onHeightChange` would violate AC-4 by yanking users who deliberately paused
sticky scroll. A dedicated `repinToBottom` that respects the preference but
ignores the height guard is the minimal correct primitive.

## Key Decisions

- **Fix in shared `EmbeddedSessionView`, drive from the parent handler.** The
  transcript owns scroll state; the composer only knows "my height changed." The
  parent already bridges them via `onHeightChange` + `sessionViewRef`, so we
  extend that seam rather than teaching the composer about scroll internals.
- **Occlusion is the primary bug; disengage is treated as its symptom.** Pinning
  removes the user's reason to scroll. We also add an implementation task to
  reproduce the disengage and confirm no automatic unlock path remains (Open
  Question 1).
- **No timing hacks.** `RobustPromptInput` already delays its queue-change
  `onHeightChange` by ~50ms to let the Collapse animation start; `repinToBottom`
  reads live `scrollHeight` at call time, so it picks up the grown layout without
  additional `setTimeout` in `EmbeddedSessionView`.

## Testing

Per project rules, verify end-to-end in the inner Helix (`localhost:8080`):
create a spec task, open its detail page, and in the chat panel:

1. With auto-scroll ON, send a short message → transcript stays pinned to the
   bottom; queued message + reply fully visible (AC-1).
2. Send a **large multi-line** prompt → still pinned, tail visible, auto-scroll
   stays ON without any manual scroll (AC-2, AC-5).
3. Confirm the auto-scroll toggle state is unchanged across several sends (AC-3).
4. Turn auto-scroll OFF, scroll up, send a message → view stays put, lock stays
   OFF, "Jump to latest" governs catching up (AC-4).
5. Repeat 1–4 in the mobile chat view (narrow viewport) (AC-6).
6. Confirm genuine wheel/touch scroll-up still disengages auto-scroll, and the
   initial-open force-scroll still lands on the newest message (AC-7).
