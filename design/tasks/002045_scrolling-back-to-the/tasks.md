# Implementation Tasks: Re-enable Auto-Scroll When User Scrolls Back to Bottom

- [x] In `frontend/src/components/session/EmbeddedSessionView.tsx`, add a new `lastScrollTopRef = useRef(0)` alongside the other scroll-tracking refs (`upwardAccumRef`, `lastWheelTsRef`, `touchStartYRef`, `lastTouchYRef`) around line 117-123
- [x] Extend the session-change reset effect (lines ~248-273) to also reset `lastScrollTopRef.current = 0` alongside the other ref resets
- [x] Extend `handleScroll` (~line 144): read `prevScrollTop` from `lastScrollTopRef`, update the ref to current `scrollTop`, then — only when `autoScrollRef.current` is false AND `isNearBottom()` is true AND `currScrollTop > prevScrollTop` — call `setAutoScroll(true)`, set `autoScrollRef.current = true`, and reset `upwardAccumRef.current = 0`. Continue to call `setHasNewBelow(false)` whenever we're near the bottom (existing behaviour preserved)
- [x] In `scrollToBottom` (~line 153), after the existing `container.scrollTop = container.scrollHeight` write, add `lastScrollTopRef.current = container.scrollHeight` so the subsequent `onScroll` event sees no delta (prevents AC-4 spurious re-enable on initial mount with off-preference)
- [x] In the ResizeObserver's auto-scroll-on-growth branch (~lines 315-318), add `lastScrollTopRef.current = container.scrollHeight` after the existing `container.scrollTop = container.scrollHeight` write
- [x] In `handleLoadOlder`'s viewport-preserve write (~lines 462-463), add `lastScrollTopRef.current = containerRef.current.scrollTop` after `containerRef.current.scrollTop += newScrollHeight - prevScrollHeight` (prevents AC-5 spurious re-enable on pagination)
- [~] `cd frontend && yarn build` — confirm zero TypeScript / build errors
- [ ] Verify in the inner Helix at `http://localhost:8080` (register/login as `test@helix.ml` / `helixtest` per `CLAUDE.md`, complete onboarding, open a session detail page with `EmbeddedSessionView`):
  - [ ] AC-1 wheel: pause auto-scroll, wheel up to confirm OFF, wheel back down to bottom — toggle goes filled-primary, `localStorage.helix.autoScroll === "true"`
  - [ ] AC-1 scrollbar drag: same but drag the scrollbar handle to the bottom
  - [ ] AC-1 keyboard `End`: focus the chat container, press `End` — re-enables
  - [ ] AC-3: pause auto-scroll, scroll up mid-conversation, collapse a tool-call block — auto-scroll stays OFF
  - [ ] AC-4: set `localStorage.helix.autoScroll = "false"`, reload — lands at bottom but preference stays OFF and pill appears for new content
  - [ ] AC-5: pause auto-scroll, click "Show N older messages" — preference stays OFF, viewport preserved
  - [ ] AC-6: re-enable via scroll-down, immediately wheel up ≥100px — auto-scroll flips OFF cleanly
- [ ] Open PR with conventional-commit subject `fix(session): scrolling back to bottom re-enables auto-scroll`, body referencing this spec task; push to a `feature/002045-...` branch
- [ ] Watch Drone CI on the PR (`gh pr checks <num>` or Drone MCP tools) and address any failures before handing back to the user
