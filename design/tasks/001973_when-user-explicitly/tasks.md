# Implementation Tasks

- [x] Add `USER_SCROLL_UNLOCK_PX = 100` constant in `frontend/src/hooks/useAutoScrollPreference.ts` and export it.
- [x] In `frontend/src/components/session/EmbeddedSessionView.tsx`, add refs: `upwardAccumRef`, `lastWheelTsRef`, `touchStartYRef`, `lastTouchYRef`.
- [x] Add a `useEffect` that attaches `wheel`, `touchstart`, `touchmove`, `touchend`, `touchcancel` listeners (all passive) to `containerRef.current` and detaches on cleanup.
- [x] Implement the wheel handler: bail if `!autoScrollRef.current`; reset accumulator on direction change or after 500ms gap; accumulate upward delta; call `setAutoScroll(false)` and reset when `>= USER_SCROLL_UNLOCK_PX`.
- [x] Implement the touch handlers: track finger Y delta (finger-down = content-up), accumulate, threshold-trigger identical to wheel; reset on touchend / touchcancel.
- [x] Add `lastScrolledHeightRef = useRef(0)` to `EmbeddedSessionView`. In `scrollToBottom`, when `force === false`, bail if `container.scrollHeight === lastScrolledHeightRef.current`; otherwise update the ref after the write.
- [x] In the ResizeObserver callback's `if (autoScrollRef.current) { ... }` branch, also update `lastScrolledHeightRef.current = container.scrollHeight` after the scroll write to keep both paths in sync.
- [x] Reset `lastScrolledHeightRef`, `upwardAccumRef`, `lastWheelTsRef`, and touch refs on session change (alongside the existing `hasInitiallyScrolled` reset).
- [x] Update the docstring at the top of `EmbeddedSessionView.tsx` (the "Auto-scroll model" comment, lines 57-71) to document (a) explicit user scroll-up of ≥ 100px now flips the preference OFF, and (b) `scrollToBottom()` is now a no-op when scrollHeight hasn't changed since the last write.
- [x] `cd frontend && yarn build` passes.
- [ ] Manual test: open a session with a quiet (non-streaming) interaction history. Add a temporary `console.log` inside `scrollToBottom` before the `scrollTop` write. Wait through several 3s polls. Confirm zero log lines (proves the gate works and polling no longer triggers scroll writes).
- [ ] Manual test: send a new message, watch the session stream tokens. Confirm scroll writes occur as content grows (gate correctly allows real growth through). Remove the temporary log before committing.
- [ ] Manual test on Chromium desktop: scroll up with mouse wheel ≥ 100px → toggle flips to OFF, jump-to-latest pill behavior takes over.
- [ ] Manual test on Chromium desktop: small wheel-up of 30px → toggle stays ON, content keeps auto-scrolling.
- [ ] Manual test that programmatic auto-scroll itself (visible while a session is streaming with auto-scroll ON) does NOT flip the toggle OFF.
- [ ] Manual test on iOS Safari (or Chrome devtools mobile emulation as a fallback): finger-drag down ≥ 100px flips to OFF; small flicks stay ON; momentum tail does not trigger unlock.
- [ ] Commit, push helix branch (PR is created by the platform when user clicks "Open PR").
