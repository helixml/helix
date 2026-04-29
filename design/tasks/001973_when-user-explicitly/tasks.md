# Implementation Tasks

- [ ] Add `USER_SCROLL_UNLOCK_PX = 100` constant in `frontend/src/hooks/useAutoScrollPreference.ts` and export it.
- [ ] In `frontend/src/components/session/EmbeddedSessionView.tsx`, add refs: `upwardAccumRef`, `lastWheelTsRef`, `touchStartYRef`, `lastTouchYRef`.
- [ ] Add a `useEffect` that attaches `wheel`, `touchstart`, `touchmove`, `touchend`, `touchcancel` listeners (all passive) to `containerRef.current` and detaches on cleanup.
- [ ] Implement the wheel handler: bail if `!autoScrollRef.current`; reset accumulator on direction change or after 500ms gap; accumulate upward delta; call `setAutoScroll(false)` and reset when `>= USER_SCROLL_UNLOCK_PX`.
- [ ] Implement the touch handlers: track finger Y delta (finger-down = content-up), accumulate, threshold-trigger identical to wheel; reset on touchend / touchcancel.
- [ ] Update the docstring at the top of `EmbeddedSessionView.tsx` (the "Auto-scroll model" comment, lines 57-71) to document that explicit user scroll-up of ≥ 100px now also flips the preference OFF, alongside the toggle button.
- [ ] Manual test on Chromium desktop: scroll up with mouse wheel ≥ 100px → toggle flips to OFF, jump-to-latest pill behavior takes over.
- [ ] Manual test on Chromium desktop: small wheel-up of 30px → toggle stays ON, content keeps auto-scrolling.
- [ ] Manual test that programmatic auto-scroll itself (visible while a session is streaming with auto-scroll ON) does NOT flip the toggle OFF.
- [ ] Manual test on iOS Safari (or Chrome devtools mobile emulation as a fallback): finger-drag down ≥ 100px flips to OFF; small flicks stay ON; momentum tail does not trigger unlock.
- [ ] `cd frontend && yarn build` passes.
- [ ] Commit, push, open PR against `helixml/helix`, paste full PR URL.
