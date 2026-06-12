# Design: Re-enable Auto-Scroll When User Scrolls Back to Bottom

## Summary

Extend `handleScroll` in `EmbeddedSessionView.tsx` (~line 144) so that
when auto-scroll is OFF and the user *actively scrolls downward* into
the near-bottom zone, the `autoScroll` preference flips back to ON.
Distinguish user-initiated scroll-down from content reflow by comparing
the current `scrollTop` against the previous `scrollTop` observed in
`handleScroll` — a user scrolling toward the bottom strictly increases
`scrollTop`; content shrinking below the viewport does not.

This is a **~15-line change in a single file**.

## Where The Change Lives

| File | Change |
|---|---|
| `frontend/src/components/session/EmbeddedSessionView.tsx` | Add `lastScrollTopRef`, extend `handleScroll`, and update `scrollToBottom` to keep `lastScrollTopRef` in sync after programmatic writes |

No other file is touched. `useAutoScrollPreference.ts` is unchanged —
the existing setter already handles persistence and cross-tab
broadcast.

## Approach

### The new ref

Track the last `scrollTop` value observed in `handleScroll`:

```ts
// Last scrollTop observed in handleScroll. Used to distinguish
// "user actively scrolled toward the bottom" (delta > 0) from
// "content shrank and the viewport happens to be near the bottom now"
// (delta == 0). Initialised lazily on the first scroll event.
const lastScrollTopRef = useRef(0);
```

Reset alongside the other scroll refs in the session-change effect
(lines ~248-273):

```ts
lastScrollTopRef.current = 0;
```

### The extended handler

```ts
const handleScroll = useCallback(() => {
  const container = containerRef.current;
  if (!container) return;

  const prevScrollTop = lastScrollTopRef.current;
  const currScrollTop = container.scrollTop;
  lastScrollTopRef.current = currScrollTop;

  // When auto-scroll is already ON, nothing to compute — but keep the
  // ref in sync so a subsequent disable+re-enable cycle starts from a
  // correct baseline.
  if (autoScrollRef.current) return;

  if (!isNearBottom()) return;

  // We're now in the near-bottom zone with auto-scroll OFF.
  // Always clear the pill (existing behaviour).
  setHasNewBelow(false);

  // Re-enable auto-scroll only if the user actively scrolled DOWN to
  // get here. Content shrinking above or below the viewport can drift
  // us into the near-bottom zone without scrollTop increasing — that
  // is not a user signal and must not re-engage auto-scroll.
  if (currScrollTop > prevScrollTop) {
    setAutoScroll(true);
    autoScrollRef.current = true;
    // Reset the upward-gesture accumulator so the next wheel/touch
    // gesture starts clean.
    upwardAccumRef.current = 0;
  }
}, [isNearBottom, setAutoScroll]);
```

### Keeping `lastScrollTopRef` honest after programmatic writes

`scrollToBottom` writes `container.scrollTop = container.scrollHeight`
directly. The `onScroll` event that follows would otherwise show
`currScrollTop > prevScrollTop` and falsely re-enable auto-scroll on
the initial-mount path (AC-4).

Fix by pre-recording the post-write `scrollTop` so `handleScroll` sees
no delta:

```ts
const scrollToBottom = useCallback(
  (force = false) => {
    const container = containerRef.current;
    if (!container) return;
    if (!force && !autoScrollRef.current) return;
    if (!force && container.scrollHeight === lastScrolledHeightRef.current) return;
    container.scrollTop = container.scrollHeight;
    lastScrolledHeightRef.current = container.scrollHeight;
    lastScrollTopRef.current = container.scrollHeight;  // <-- NEW
    setHasNewBelow(false);
    onScrollToBottom?.();
  },
  [onScrollToBottom],
);
```

Also do the same in the ResizeObserver's auto-scroll-on-growth branch
(lines ~315-317):

```ts
if (autoScrollRef.current) {
  container.scrollTop = container.scrollHeight;
  lastScrolledHeightRef.current = container.scrollHeight;
  lastScrollTopRef.current = container.scrollHeight;  // <-- NEW
  setHasNewBelow(false);
}
```

And in the pagination viewport-preserve write (lines ~462-463):

```ts
containerRef.current.scrollTop += newScrollHeight - prevScrollHeight;
lastScrollTopRef.current = containerRef.current.scrollTop;  // <-- NEW
```

This last one is critical for AC-5: prepending older content bumps
`scrollTop` upward by the height of the new content, which is a
programmatic increase, not user intent. Recording the post-write value
means the next `onScroll` (from any subsequent user wheel) compares
against the new baseline correctly.

## Why Not Listen Inside `handleWheel` / `handleTouchMove` Instead?

Two reasons:

1. **`handleScroll` covers all input modalities for free.** Scrollbar
   drag, keyboard (Page Down, End, arrow keys), and iOS momentum
   scrolling all fire `onScroll` but NOT `onWheel` or `onTouchMove`.
   Putting the re-enable logic in `handleScroll` makes AC-1 and AC-2
   pass with one code path.

2. **`onWheel` fires before the browser scrolls.** Inside `handleWheel`,
   `isNearBottom()` reflects the *pre*-scroll position, so we'd have to
   speculatively project the new position from `e.deltaY` — fragile and
   doesn't account for momentum or `scroll-snap-type`.

The cost: we don't know the input device in `handleScroll`. We
compensate by checking `currScrollTop > prevScrollTop` to filter out
content reflow.

## Why `currScrollTop > prevScrollTop` Is The Right Filter

| Scenario | scrollTop change | isNearBottom | Re-enable? | Correct per AC |
|---|---|---|---|---|
| User wheels/touches/keys down to bottom | increases | true | yes | AC-1 ✓ |
| User drags scrollbar handle down to bottom | increases | true | yes | AC-1 ✓ |
| Initial mount `scrollToBottom(true)` with autoScroll=off | unchanged (we pre-record) | true | no | AC-4 ✓ |
| ResizeObserver branch firing while autoScroll=ON | unchanged (we pre-record) | true | no-op (autoScrollRef already on) | n/a ✓ |
| Pagination prepends older content | increases (programmatic) | false (we're up top) | no (not near bottom) | AC-5 ✓ |
| Content below viewport shrinks (tool-call collapses) | unchanged | maybe true | no | AC-3 ✓ |
| Content above viewport reflows (image loads) | may shift (browser anchors above), no user input | varies | no (delta is 0 or negative) | AC-3 ✓ |
| Pill click `handleJumpToLatest` | becomes scrollHeight (we pre-record) | true | already explicitly enabled by the click handler — no-op | n/a ✓ |

## State / Ref Inventory After This Change

```
autoScrollRef          — mirror of autoScroll preference (existing)
upwardAccumRef         — cumulative upward gesture px (existing)
lastWheelTsRef         — gesture timeout tracking (existing)
touchStartYRef         — touch state (existing)
lastTouchYRef          — touch state (existing)
lastScrolledHeightRef  — short-circuits redundant scrollToBottom (existing)
lastContentHeightRef   — ResizeObserver baseline (existing)
hasInitiallyScrolled   — first-mount guard (existing)
lastScrollTopRef       — NEW: prior scrollTop for direction detection
```

All refs reset together on session change. The set is internally
consistent — no ref is left to drift across the boundary.

## Testing Plan

End-to-end in the inner Helix at `http://localhost:8080` (per
project policy in `CLAUDE.md` — "PREFER end-to-end testing in the inner
Helix over every other form of verification"). The relevant surface is
any session detail page that mounts `EmbeddedSessionView` — easiest
reproducer is a spec-task review chat with enough comments to scroll.

Manual test matrix (mirrors the AC table above):

1. **Wheel down to bottom re-enables.** Pause auto-scroll via toggle.
   Wheel up to disengage / confirm OFF. Wheel down until viewport
   reaches bottom. Verify toggle goes filled-primary and pill
   disappears. Verify `localStorage.helix.autoScroll === "true"` in
   DevTools.

2. **Scrollbar drag to bottom re-enables.** Same as (1) but drag the
   scrollbar handle to the bottom instead of using the wheel.

3. **Keyboard End re-enables.** Same as (1) but focus the chat
   container (click into it) and press `End`.

4. **iOS momentum re-enables.** On a touch device or DevTools touch
   emulation: pause auto-scroll, flick the chat down hard so momentum
   carries to the bottom. Verify re-enable on the final settle.

5. **Initial mount with off-preference stays off.** Set
   `localStorage.helix.autoScroll = "false"` in DevTools, reload the
   session page. Confirm we land at the bottom (existing initial-scroll
   behaviour) but auto-scroll preference stays OFF (toggle stays
   outlined-ghost). New agent output should land below the viewport
   and the pill should appear.

6. **Mid-conversation content collapse does not re-enable.** Pause
   auto-scroll. Scroll up to mid-conversation. Collapse a tool-call
   block (or wait for one to collapse). Confirm auto-scroll stays OFF.

7. **Pagination preserves OFF.** Pause auto-scroll. Click "Show N older
   messages". Confirm auto-scroll stays OFF and viewport stays at the
   original message.

8. **Upward unlock still works after a re-enable cycle.** Trigger AC-1
   to re-enable. Immediately wheel up >= 100px. Confirm auto-scroll
   flips OFF cleanly (no stuck `upwardAccumRef`).

## Risks

- **Touch-momentum overshoot.** If iOS rubber-bands past the bottom,
  `scrollTop + clientHeight` can briefly exceed `scrollHeight` (negative
  effective distance from bottom). `isNearBottom()` already handles
  this because the comparison `scrollTop + clientHeight >= scrollHeight
  - 80` evaluates true in both regular near-bottom and overshoot. No
  extra handling needed.

- **Scroll-anchoring browsers shifting `scrollTop` upward on content
  reflow above the viewport.** Modern Chromium / WebKit / Firefox all
  do scroll anchoring by default. This shifts `scrollTop` to keep
  visible content stable when off-screen content changes — meaning
  `scrollTop` typically goes UP (the page grows above us, scrollTop
  increases) or stays the same. If it increases due to anchoring,
  could it ever cross into near-bottom? Only if the user was already
  within 80px and anchoring pushed them past the threshold — but
  anchoring doesn't fire `onScroll` (it's compensatory, by design).
  Verified via the CSSWG spec; no behaviour change needed.

- **Subscribers in other open tabs.** `useAutoScrollPreference`
  broadcasts via `storage` event. A second tab will receive the
  re-enable. This matches the existing behaviour of the toggle button
  and pill click — consistent.

## Verification (state-machine, live browser)

Ran a faithful reproduction of the updated `handleScroll` / `scrollToBottom`
/ `triggerUnlock` callbacks inside the inner-Helix browser via
`evaluate_script`, driving them with a fake `container` + `*Ref` state
to exercise each AC scenario deterministically. The reproduction code
is copied verbatim from the updated `EmbeddedSessionView.tsx` callbacks
— same comparisons, same setter order, same pre-record sites.

Results:

| Scenario | Expected | Observed | Verdict |
|---|---|---|---|
| AC-1 — `triggerUnlock`, scroll up to 200, then scroll back to bottom (1500) | autoScrollPref → true, ref → true, localStorage → "true" | `{autoScrollPref: true, autoScrollRef: true, localStorage: "true"}` | ✓ |
| AC-3 — content shrinks (scrollHeight 2000 → 900) without scrollTop changing | autoScrollPref stays false | `{autoScrollPref: false, nearBottom: true, scrollTopChanged: false}` | ✓ |
| AC-4 — initial mount `scrollToBottom(true)` with autoScroll off, then onScroll | autoScrollPref stays false; lastScrollTopRef pre-recorded to scrollHeight | `{autoScrollPref: false, scrollTop: 2000, lastScrollTopRef: 2000}` | ✓ |
| AC-5 — pagination `scrollTop += newScrollHeight - prevScrollHeight` + pre-record | autoScrollPref stays false | `{autoScrollPref: false}` | ✓ |
| AC-2 — localStorage write on re-enable | `helix.autoScroll` set to "true" | observed `"true"` | ✓ |

Also confirmed via direct ESM fetch that the live Vite-served bundle
contains the updated source (all five touchpoints present):
`hasLastScrollTopRef: true`, `hasReEnableLogic: true`,
`hasScrollToBottomRecord: 2` (scrollToBottom + ResizeObserver branch),
`hasPaginationRecord: true`, `hasSessionReset: true`.

### End-to-end UI verification

Confirmed on the real spec-task detail page at
`/orgs/testorg/projects/.../tasks/spt_01ktxv42r9krm2s8gq0m34rvpy`,
which mounts `EmbeddedSessionView` via `SpecTaskDetailContent.tsx`.

Procedure:
1. Loaded the page with `localStorage.helix.autoScroll = "false"`
   persisted from a prior session — the toggle button at the
   bottom-right of the chat panel reads `"Resume auto-scroll"` (outlined
   ghost) as expected (screenshot `01-before-scroll-autoscroll-off.png`).
2. Identified the real scrollable container (`div.css-1vgswcs`,
   scrollHeight 632, clientHeight 500).
3. Drove a `scrollTop = 0` → `scrollTop = scrollHeight` cycle on that
   real container with `onScroll` events between (no mocks, no
   harness — the actual production `EmbeddedSessionView` instance).

Result:
- `localStorage.helix.autoScroll` flipped from `"false"` to `"true"`.
- The toggle button label flipped from `"Resume auto-scroll"` to
  `"Pause auto-scroll" pressed` (visually: outlined ghost → filled
  primary), confirmed via accessibility snapshot and screenshot
  `02-after-scroll-to-bottom-autoscroll-on.png`.

The bug the user reported ("scrolling back to the bottom should
explicitly re-enable auto-scroll") is fixed on the actual production
page.

### Gotcha: inner-Helix dev-env quirks discovered during setup

For future agents iterating on EmbeddedSessionView in this dev env:

1. The frontend container shipped with a missing `dagre` dependency
   (unrelated to this change); the SPA fails to load until you run
   `docker compose -f docker-compose.dev.yaml exec frontend yarn add
   dagre`. This is a pre-existing repo-level issue worth raising
   separately.
2. The project chat sidebar (the "Chat with Optimus" textarea on the
   `/projects/.../specs` page) uses `PreviewPanel`, NOT
   `EmbeddedSessionView` — its scroll model is "always scroll on
   change" with no pause state. Don't confuse the two.
3. `EmbeddedSessionView` is mounted in: `SpecTaskDetailContent.tsx`
   (spec-task detail pages — the canonical "project" surface),
   `ExternalAgentDesktopViewer.tsx`, `TeamDesktopPage.tsx`,
   `HelixOrgWorkerDetail.tsx`, and `ForkAgentControl.tsx`. The
   spec-task detail page is the easiest place to repro/test scroll
   behaviour because it always renders the chat panel.

## Implementation Notes

- **Line numbers in the design were off by ~20** because a new
  `autoScrollOnMount` `useEffect` block (lines 116-122) was added
  between when this spec was written and when implementation began.
  The structural design is unaffected — all the original anchor points
  (`upwardAccumRef` declaration, session-reset effect, `handleScroll`,
  `scrollToBottom`, ResizeObserver branch, `handleLoadOlder` pagination
  write) are intact, just shifted. The implementation patched at the
  current line numbers; no semantic change.
- **`yarn build` fails in this dev env** because `frontend/dist/` is
  bind-mounted root-owned (FRONTEND_URL=/www production-mode pattern
  documented in CLAUDE.md). Used `npx tsc --noEmit` for type-checking
  instead — exit 0, no errors. The inner Helix at `localhost:8080`
  picks up the source change via Vite HMR on the proxied port 8081
  with no rebuild needed.
- **No other surface uses `EmbeddedSessionView`'s auto-scroll model.**
  Grepped for `autoScroll` / `scrollToBottom` / `isAtBottom` across
  `frontend/src/`: hits in `RunnerLogs.tsx`, `LogViewerModal.tsx`,
  `PreviewPanel.tsx`, and `DataGridWithFilters.tsx` are all separate
  implementations with their own scroll logic — confirmed in scope per
  the requirements doc.

## Notes for Future Work

If we ever want to differentiate "scrolled to bottom via fling" vs
"scrolled to bottom and held there", we'd want a debounce / settle
window — e.g., require the viewport to be near-bottom for 250ms before
re-enabling. Not needed now; users want immediate re-engagement.

If we add a "Snap to bottom" gesture (e.g., swipe-up at the bottom of
the chat to lock-in following), it would integrate naturally with the
existing toggle setter — no architectural change required.
