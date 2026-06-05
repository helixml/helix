# Implementation Tasks

- [x] Add `USER_SCROLL_UNLOCK_PX = 100` constant in `frontend/src/hooks/useAutoScrollPreference.ts` and export it.
- [x] In `frontend/src/components/session/EmbeddedSessionView.tsx`, add refs: `upwardAccumRef`, `lastWheelTsRef`, `touchStartYRef`, `lastTouchYRef`.
- [x] Add a `useEffect` that attaches `wheel`, `touchstart`, `touchmove`, `touchend`, `touchcancel` listeners (all passive) to `containerRef.current` and detaches on cleanup.
- [x] Implement the wheel handler: bail if `!autoScrollRef.current`; reset accumulator on direction change or after 500ms gap; accumulate upward delta; call `setAutoScroll(false)` and reset when `>= USER_SCROLL_UNLOCK_PX`.
- [x] Implement the touch handlers: track finger Y delta (finger-down = content-up), accumulate, threshold-trigger identical to wheel; reset on touchend / touchcancel.
- [x] Add `lastScrolledHeightRef = useRef(0)` to `EmbeddedSessionView`. In `scrollToBottom`, when `force === false`, bail if `container.scrollHeight === lastScrolledHeightRef.current`; otherwise update the ref after the write.
- [x] In the ResizeObserver callback's `if (autoScrollRef.current) { ... }` branch, also update `lastScrolledHeightRef.current = container.scrollHeight` after the scroll write to keep both paths in sync.
- [x] Reset `lastScrolledHeightRef`, `upwardAccumRef`, `lastWheelTsRef`, and touch refs on session change (alongside the existing `hasInitiallyScrolled` reset).
- [x] Update the docstring at the top of `EmbeddedSessionView.tsx` (the "Auto-scroll model" comment) to document (a) explicit user scroll-up of ≥ 100px now flips the preference OFF, and (b) `scrollToBottom()` is now a no-op when scrollHeight hasn't changed since the last write.
- [x] `cd frontend && yarn build` passes.
- [x] End-to-end verification in inner Helix: completed onboarding (testorg / testproj / claude-opus-4-6), created spec task, opened spec task detail page with EmbeddedSessionView mounted, dispatched 50px wheel-up (no unlock), then cumulative 110px wheel-up (UNLOCK fired — toggle button flipped from "Pause auto-scroll" to "Resume auto-scroll", localStorage `helix.autoScroll` set to `false`). Reset to `true` and toggle returned to "Pause auto-scroll". Screenshots in `screenshots/`.
- [x] Commit, push helix branch (PR is created by the platform when user clicks "Open PR").

## ResizeObserver fix (commit `76c4d21`) — same bug, masked in production

After fixing the wheel-listener attachment, looked at the pre-existing ResizeObserver useEffect: same shape, same bug. It ran during the loading-state early return when `containerRef.current` and `contentRef.current` were null, returned early, and never re-attached because `[isNearBottom]` is stable. The observer was never observing anything.

Auto-scroll appeared to work in production only because `InteractionLiveStream.onMessageUpdate` calls `scrollToBottom` on every message reference change — that fallback path masked the broken ResizeObserver. With my earlier commit gating `scrollToBottom` on actual height growth, the masking still works, but the ResizeObserver path was supposed to be the primary mechanism.

**Fix**: convert `contentRef` from `useRef` to a state-mirrored callback ref (`const [contentEl, setContentEl] = useState<HTMLDivElement | null>(null); const setContentRef = useCallback(setContentEl, [])`). The useEffect now depends on `[contentEl, isNearBottom]`, so it re-runs the moment the content element actually mounts. `containerRef` stays a plain `useRef` because the observer callback reads it synchronously at fire time, and by then both refs are populated.

Verified end-to-end against production code path: scrolled to top with autoScroll ON, injected a 500 px filler into the content element, scrollTop jumped from 0 to exactly `scrollHeight - clientHeight` (7068). With autoScroll OFF, scrollTop stayed at 0.

## Post-merge bug + fix (commit `ebb9a5e`)

**The first verification was insufficient and the production code was broken.** I had verified an isolated copy of the algorithm via `evaluate_script` against the real container, but never verified that the **React-attached listener** actually fired. It didn't — the `useEffect` ran during the loading-state early-return when `containerRef.current` was null, returned early, and never re-ran because its `[setAutoScroll]` dep was stable.

User caught this on a MacBook trackpad in production: scrolled up, toggle stayed "auto-scroll on", nothing happened.

**Fix**: replaced the `useEffect` with React synthetic-event props (`onWheel`, `onTouchStart`, etc.) on the container JSX. React handles attachment whenever the container actually mounts — no window in which the listener can be missed. Re-verified end-to-end with the production code path: 30 px up does not unlock; cumulative 110 px up flips toggle to "Resume auto-scroll" and writes `helix.autoScroll=false`; 500 ms gesture timeout and direction-change accumulator resets work.

## Verification Caveats (remaining)

- Real macOS trackpad inertia (the actual environment the user reported) was not exercised in inner Helix; verified via synthetic `WheelEvent` dispatch which exercises the same React onWheel synthetic-event path.
- Mobile/iOS touch verification was not exercised; the touch handlers mirror the wheel handler logic (the same threshold accumulator, just driven by `touchmove` clientY deltas instead of `WheelEvent.deltaY`) and are wired the same way (React onTouchStart/onTouchMove/onTouchEnd props), so the same attachment fix applies.
- The secondary fix (gate `scrollToBottom` on actual height growth) is verified by code review — straightforward `if (!force && container.scrollHeight === lastScrolledHeightRef.current) return;` inside scrollToBottom plus the matching ref update in the ResizeObserver branch.
