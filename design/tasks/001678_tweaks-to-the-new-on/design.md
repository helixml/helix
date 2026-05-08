# Design: Tweaks to On-Hover Add Comment Button

All changes are in `frontend/src/components/spec-tasks/DesignReviewContent.tsx`.

## 1. Highlight text on hover-button click

**Root cause:** The hover button's `onClick` never sets `savedRangeRef.current`, so the `useEffect` that calls `applyHighlight(savedRangeRef.current)` when `showCommentForm` becomes true finds a null ref and skips highlighting.

**Fix:** Before setting `showCommentForm = true`, create a `Range` from the hovered element using `document.createRange()` + `range.selectNodeContents(hoveredElementRef.current)`, and store it in `savedRangeRef.current`. The existing `useEffect` then applies the highlight automatically.

```typescript
onClick={() => {
  if (hoveredElementRef.current) {
    const range = document.createRange();
    range.selectNodeContents(hoveredElementRef.current);
    savedRangeRef.current = range;
  }
  setSelectedText(hoverButtonPosition.elementText);
  setCommentFormPosition({ x: 0, y: hoverButtonPosition.y });
  setHoverButtonPosition(null);
  setShowCommentForm(true);
}}
```

No other changes needed — the existing `useEffect`/`applyHighlight`/`removeHighlight` flow handles the rest.

## 2. Hide button when cursor moves past its right edge

**Root cause:** The `onMouseMove` handler is on the inner document `Box`, but the hover button is rendered outside it (as a sibling in the outer scroll container). When the cursor moves over or past the button, the inner `Box`'s `onMouseMove` doesn't fire, so `hoverButtonPosition` is never cleared.

**Fix:** Add an `onMouseMove` handler to the outer scroll container (the Box with `onMouseLeave`) that computes the button's right edge in container-relative coordinates and clears `hoverButtonPosition` when the cursor is past it.

The button is positioned at `left: calc(50% + 400px + 4px)` with width 28px, so its right edge is at container x = `containerWidth/2 + 432px`.

```typescript
// On the outer Box:
onMouseMove={(e) => {
  if (!hoverButtonPosition) return;
  const containerRect = (e.currentTarget as HTMLElement).getBoundingClientRect();
  const mouseX = e.clientX - containerRect.left;
  const buttonRightEdge = containerRect.width / 2 + 400 + 4 + 28;
  if (mouseX > buttonRightEdge) {
    setHoverButtonPosition(null);
    hoveredElementRef.current = null;
  }
}}
```

This only triggers when `hoverButtonPosition` is set (button is visible) and cursor has moved past the right edge. Clicking the button is unaffected because on click the cursor is over the button, not past it.

## 3. Fix pseudo-highlight truncation spanning code blocks

**Update during implementation:** Discovery showed `helix` already migrated `applyHighlight` to use the **CSS Custom Highlight API** (`CSS.highlights` + `new Highlight(range)`) — see `DesignReviewContent.tsx:879-899` and the GlobalStyles rule at line 1116. This is non-destructive (no DOM mutation), so the original `extractContents` problem from `helix-4` doesn't apply here.

**Investigation outcome:** Chrome's CSS Custom Highlight API has a known limitation — `::highlight() { color: ... }` does NOT reliably override inherited text color. Pixel-level inspection (`screenshots/11-zoomed-highlight-text.png`) confirmed that text inside the highlight stayed white in dark mode despite the `color: #000` rule being present in the registered CSS rule.

**False-start fixes (both reverted):**
1. First attempt: dropped `color: #000` — broke dark mode (white text on light blue, illegible).
2. Second attempt: restored `color: #000` — same dark-mode bug, because Chrome ignores the override.

**Final fix:** Use a translucent saturated blue (`rgba(25, 118, 210, 0.4)`) for the background and **no** `color` override. The translucency lets the underlying page colors show through, so:
- Light mode (dark text on light Paper): highlight tints to a softer light blue; dark text remains legible.
- Dark mode (white text on dark Paper): highlight tints to a darker blue; white text remains legible against it.
- Code blocks: highlight paints across each text line; Prism syntax token colours show through unchanged.

**Verified** in both light (`screenshots/13-translucent-lightmode.png`, `14-translucent-codeblock-lightmode.png`) and dark (`screenshots/12-translucent-test.png`, `16-translucent-darkmode-actual.png`) modes — highlight visible and text legible across normal paragraphs and code blocks.

## 4. No hover button when cursor is over a comment panel

**Root cause:** `InlineCommentBubble` panels are rendered as siblings of the markdown `<Paper>` inside the same `onMouseMove` Box, but they are **not** descendants of `markdownRef.current`. When the cursor moves over a bubble, the `onMouseMove` walk goes up through the bubble's DOM tree, never reaches `markdownRef.current`, and exits the while loop with `node === null` — at which point the handler does nothing, leaving the last `hoverButtonPosition` stale and the button still visible.

**Fix:** At the top of the `onMouseMove` handler (before the while loop), check whether `e.target` is contained within any comment bubble using `commentRefs.current`. If so, clear the hover button and return early.

```typescript
onMouseMove={(e) => {
  if (showCommentForm || isNarrowViewport) return;
  // Clear button if cursor enters a comment panel
  const isOverBubble = Array.from(commentRefs.current.values()).some(
    (el) => el.contains(e.target as Node)
  );
  if (isOverBubble) {
    setHoverButtonPosition(null);
    hoveredElementRef.current = null;
    return;
  }
  // ... existing block-tag walk
}}
```

`commentRefs` is already a `useRef<Map<string, HTMLDivElement>>` populated via the `commentRef` callback prop on each `InlineCommentBubble`, so no new state is needed.
