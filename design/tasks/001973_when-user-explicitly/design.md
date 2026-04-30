# Design

## Files Touched

- `frontend/src/components/session/EmbeddedSessionView.tsx` — primary change. Add user-input listeners and accumulator. Also gate `scrollToBottom()` on actual scrollHeight growth.
- `frontend/src/hooks/useAutoScrollPreference.ts` — add a new exported constant `USER_SCROLL_UNLOCK_PX = 100`.

That's it. No new files, no new hooks. Spec task detail page (`SpecTaskDetailContent.tsx`) does not change — it just renders `EmbeddedSessionView`, which is where the scroll happens.

## Two Independent Changes In One Pass

This task addresses two related but independent issues in the same component. Both are small, both touch the same auto-scroll machinery, and bundling them avoids two consecutive PRs against the same 50-line region.

| # | Issue | Change |
|---|---|---|
| 1 | (primary) No way to disengage auto-scroll without clicking the toggle button | Add wheel/touch listeners that flip `autoScroll` OFF after a cumulative ≥ 100px upward gesture |
| 2 | (secondary, raised in review) `scrollToBottom()` writes `scrollTop` on every poll/WS message even when nothing grew | Gate `scrollToBottom(force=false)` on `scrollHeight` changing since the last scroll write |

## Key Decision: Listen to User Input, Not Scroll Position

The prior sticky-scroll implementation (removed in `42c3a5112`) failed because it inferred user intent from `scrollTop` — which is also written by programmatic scrolls and shifted by content reflow above the viewport. Three uncoordinated triggers fed a single `isAtBottomRef` boolean and the RAF guard window was too short for Chromium's coalesced scroll events and iOS Safari momentum scrolling.

We avoid all of that by detecting user scroll from the **input events themselves**, not from scroll-position deltas:

| Event | Why it's safe |
|---|---|
| `wheel` | Only fires from real user input (mouse wheel, trackpad). Programmatic `scrollTop` writes do not synthesize wheel events. |
| `touchmove` | Only fires from a finger on the screen. iOS momentum scrolling produces `scroll` events but **not** further `touchmove`s, so we measure the user's actual finger drag, not the momentum tail. |

Programmatic scrolls and content reflow are inherently invisible to these listeners, eliminating both prior race surfaces.

## Algorithm

In `EmbeddedSessionView`, alongside the existing `autoScrollRef`:

```ts
const upwardAccumRef = useRef(0);          // cumulative px scrolled up in current gesture
const lastWheelTsRef = useRef(0);          // for gesture-end timeout
const touchStartYRef = useRef<number | null>(null);
const lastTouchYRef  = useRef<number | null>(null);
```

**Wheel handler** (passive listener on `containerRef`):
1. If `!autoScrollRef.current` → return (no-op when already off).
2. If `e.deltaY < 0` (scrolling up): add `-e.deltaY` to `upwardAccumRef`.
3. If `e.deltaY > 0` (scrolling down): reset `upwardAccumRef = 0`.
4. If `now - lastWheelTsRef > 500ms`: reset accumulator before adding (treat as new gesture).
5. Update `lastWheelTsRef = now`.
6. If `upwardAccumRef >= USER_SCROLL_UNLOCK_PX` → call `setAutoScroll(false)` and reset accumulator.

**Touch handlers** (passive listeners on `containerRef`):
- `touchstart`: record `touchStartYRef = e.touches[0].clientY`, `lastTouchYRef = same`, reset `upwardAccumRef = 0`.
- `touchmove`: compute `dy = e.touches[0].clientY - lastTouchYRef`. Finger moving **down** the screen scrolls content **up** (so `dy > 0` means user is reading older content). Add `dy` to `upwardAccumRef` when positive; subtract / reset when negative. Update `lastTouchYRef`. Threshold check identical to wheel.
- `touchend`/`touchcancel`: reset `upwardAccumRef = 0`, clear refs.

**Gesture-end reset (500ms timeout)** prevents the situation where a user scrolls up 60px, reads for 30s, then scrolls up another 60px — those should be two independent gestures, not a 120px cumulative trigger.

## Why a Threshold (vs. trigger on first wheel up)

A bare "first wheel-up = unlock" would fire on accidental trackpad jitter and on Mac inertial scrolling overshoot. 100px is large enough to require deliberate intent but small enough to feel responsive. Matched to `AUTO_SCROLL_NEAR_BOTTOM_PX = 80` in spirit — slightly larger so users near-but-not-at-bottom don't accidentally toggle.

## Listener Lifecycle

Attach the wheel and touch listeners in a `useEffect` keyed on `containerRef.current` (or just `[]` since the container ref is stable for the component's lifetime). All listeners are passive (`{ passive: true }`) — we never preventDefault. Detach on unmount.

## What We Are Deliberately NOT Doing

- **Not** tracking `scrollTop` deltas. That was the prior bug.
- **Not** adding a programmatic-scroll guard / RAF flag. Wheel and touchmove cannot be triggered by `scrollTop` writes, so no guard is needed.
- **Not** listening for `keydown` (PageUp etc.) in the first cut. Can be added later if requested; keyboard-driven scroll is rare in this view.
- **Not** changing the `useAutoScrollPreference` hook's API or the localStorage shape. We just export one new constant from that file.
- **Not** touching the "Jump to latest" pill, the toggle button, the ResizeObserver auto-scroll path, or initial-mount force-scroll. Those all keep working unchanged because they only read `autoScrollRef.current` — our change only flips that ref OFF earlier than it would otherwise flip.

## Secondary Fix: Gate `scrollToBottom` on Actual Growth

### Root cause

`InteractionLiveStream.tsx:100-115` runs an effect on `[hasContent, message, responseEntries, onMessageUpdate]`. `message` and `responseEntries` are new object/array references on every WebSocket update and every React Query poll that returns updated (even logically-identical) data. The effect throttles to `SCROLL_THROTTLE_MS` but always calls `onMessageUpdate()` (= `scrollToBottom`) at least once per update. `scrollToBottom()` then unconditionally writes `container.scrollTop = container.scrollHeight`. So a session that is producing no new content still incurs one `scrollTop` write per polling interval (3s) and one per any incoming WS message.

### Fix

Add a `lastScrolledHeightRef = useRef(0)` to `EmbeddedSessionView`. Modify `scrollToBottom`:

```ts
const scrollToBottom = useCallback(
  (force = false) => {
    const container = containerRef.current;
    if (!container) return;
    if (!force && !autoScrollRef.current) return;
    // Skip no-op writes: nothing to do if scrollHeight hasn't changed since
    // the last write. Polling and WS keepalives produce new message refs
    // without changing rendered height — those should not trigger scrolls.
    if (!force && container.scrollHeight === lastScrolledHeightRef.current) return;
    container.scrollTop = container.scrollHeight;
    lastScrolledHeightRef.current = container.scrollHeight;
    setHasNewBelow(false);
    onScrollToBottom?.();
  },
  [onScrollToBottom],
);
```

Also update `lastScrolledHeightRef.current` in the ResizeObserver path after its `container.scrollTop = container.scrollHeight` write, to keep the two paths in sync.

### Why this is safe

- **Force calls bypass the check.** Initial-mount, session-change, and the jump-to-latest pill all use `scrollToBottom(true)` and continue to scroll unconditionally.
- **Genuine content growth is unaffected.** When `scrollHeight` actually increases (new tokens from the streaming agent), the comparison fails and the scroll fires.
- **No new race surface.** `scrollHeight` is read synchronously inside the function. We're not inferring user intent from it (which was the prior bug); we're just deduplicating writes to the same target.
- **`handleRegenerate`'s `scrollToBottom()` call** (line 293) still works: it's called on user action; if scrollHeight hasn't changed yet, the ResizeObserver will pick up the next growth and scroll then.

### What we are deliberately NOT doing for the secondary fix

- **Not removing `onMessageUpdate={scrollToBottom}` from `InteractionLiveStream`.** It's a generic "message updated" callback; other future uses are imaginable. Gating `scrollToBottom` itself is the smaller, more local fix.
- **Not removing the throttle in `InteractionLiveStream`.** With the gate in place, the redundant calls cost nothing but a `scrollHeight` read; removing the throttle would only matter if the read itself were expensive (it isn't).

## Risk / Test Notes

- iOS Safari momentum scroll: verify by manual test that flicking up 50px and letting momentum scroll the rest does NOT trip the unlock (because momentum scrolling does not generate `touchmove`). This is actually the *correct* behavior — small flicks shouldn't unlock.
- Trackpad inertial scroll on macOS: this DOES generate continued `wheel` events as it decelerates. The 500ms gesture-reset timeout means a small flick stays as one gesture; if the inertial tail accumulates over 100px the unlock fires, which is what we want (the user did intentionally flick up far enough).
- Test that auto-scroll engaged via the toggle button still re-arms the listeners (it does, because they only read `autoScrollRef.current` and bail when off — flipping back on just makes the next user-up-scroll re-trigger).
