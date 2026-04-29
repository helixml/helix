# Design

## Files Touched

- `frontend/src/components/session/EmbeddedSessionView.tsx` — primary change. Add user-input listeners and accumulator.
- `frontend/src/hooks/useAutoScrollPreference.ts` — add a new exported constant `USER_SCROLL_UNLOCK_PX = 100`.

That's it. No new files, no new hooks. Spec task detail page (`SpecTaskDetailContent.tsx`) does not change — it just renders `EmbeddedSessionView`, which is where the scroll happens.

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

## Risk / Test Notes

- iOS Safari momentum scroll: verify by manual test that flicking up 50px and letting momentum scroll the rest does NOT trip the unlock (because momentum scrolling does not generate `touchmove`). This is actually the *correct* behavior — small flicks shouldn't unlock.
- Trackpad inertial scroll on macOS: this DOES generate continued `wheel` events as it decelerates. The 500ms gesture-reset timeout means a small flick stays as one gesture; if the inertial tail accumulates over 100px the unlock fires, which is what we want (the user did intentionally flick up far enough).
- Test that auto-scroll engaged via the toggle button still re-arms the listeners (it does, because they only read `autoScrollRef.current` and bail when off — flipping back on just makes the next user-up-scroll re-trigger).
