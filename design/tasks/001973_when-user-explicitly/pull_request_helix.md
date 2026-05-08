# Disengage auto-scroll on explicit user scroll-up; gate scrollToBottom on actual growth

## Summary

Two related fixes to `EmbeddedSessionView` (the chat thread inside the spec task detail page):

1. **Explicit user scroll-up disengages auto-scroll.** When `helix.autoScroll` is ON and the user wheels (or finger-drags) upward by a cumulative ≥ 100 px within a single gesture, the preference flips OFF — same effect as clicking the toggle button, but reachable from the natural reading gesture instead of hunting for the icon.
2. **`scrollToBottom()` no longer writes scrollTop on every poll/WS update.** `InteractionLiveStream`'s `onMessageUpdate` callback fires (throttled but ungated) on every message/responseEntries reference change — which happens on every 3 s React Query poll and every WS keepalive even when no content has actually grown. The unconditional `container.scrollTop = container.scrollHeight` write inside `scrollToBottom()` was firing each time. Now gated on `container.scrollHeight` having changed since the last write; force-callers (initial mount, session change, jump-to-latest pill) bypass the gate.

## Why not the previous sticky-scroll approach

The previous "sticky-scroll" implementation was deliberately removed in commit `42c3a5112` because inferring user intent from `scrollTop` deltas had three persistent race surfaces (content reflow above the viewport, RAF guard windows that were too short for Chromium coalesced scroll events / iOS momentum, and uncoordinated triggers feeding a single `isAtBottomRef`).

This PR sidesteps all three by detecting user scroll from **input events themselves** (`wheel`, `touchmove`) instead of `scrollTop`. Programmatic scrolls and content reflow can't synthesize wheel/touchmove events, so they cannot trip the unlock — by construction, not by guard.

## Changes

- `frontend/src/hooks/useAutoScrollPreference.ts` — export new `USER_SCROLL_UNLOCK_PX = 100` constant.
- `frontend/src/components/session/EmbeddedSessionView.tsx`
  - New refs: `upwardAccumRef`, `lastWheelTsRef`, `touchStartYRef`, `lastTouchYRef`, `lastScrolledHeightRef`.
  - New `useEffect` attaching passive `wheel` / `touchstart` / `touchmove` / `touchend` / `touchcancel` listeners to `containerRef.current`. Wheel handler accumulates upward delta within a 500 ms gesture window (resets on direction change or quiet gap); touch handler tracks finger-Y delta in the same shape; both call `setAutoScroll(false)` once the accumulator reaches 100 px.
  - `scrollToBottom(force=false)` now bails when `container.scrollHeight === lastScrolledHeightRef.current`. The ref is updated by `scrollToBottom` itself and by the ResizeObserver auto-scroll path so they stay in sync. All five new refs are reset on session change.
  - Updated the top-of-file "Auto-scroll model" docstring to reflect both changes.
- `CLAUDE.md` — added a note in "Never Give Up on Testing": don't skip end-to-end testing in the inner Helix on grounds of "setup feels like work."

## Test plan

- [x] `cd frontend && yarn build` passes.
- [x] End-to-end in the inner Helix spec task detail page (testorg / testproj, Claude Code session): dispatched 50 px upward wheel — toggle stays "Pause auto-scroll" / pressed (autoScroll still ON). Dispatched another 60 px upward — cumulative 110 px tripped the unlock; toggle button flipped to "Resume auto-scroll" and `localStorage.helix.autoScroll` became `"false"`. Set the preference back to `"true"` — toggle returned to "Pause auto-scroll". Screenshots in design docs.
- [ ] iOS Safari momentum scroll (manual review): a quick flick should NOT trip unlock because momentum scrolling fires `scroll` but not further `touchmove` events; long deliberate finger drags should.
- [ ] Reviewer to spot-check that streaming sessions still auto-scroll as content grows (ResizeObserver path keeps `lastScrolledHeightRef` in sync, so the gate inside `scrollToBottom` doesn't block real growth).

## Screenshots

![Spec task page, auto-scroll ON ("Pause auto-scroll" button)](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001973_when-user-explicitly/screenshots/01-spectask-page-autoscroll-on.png)
![After 110 px upward wheel — toggle flipped to "Resume auto-scroll"](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001973_when-user-explicitly/screenshots/02-after-110px-wheel-toggle-flipped-off.png)
