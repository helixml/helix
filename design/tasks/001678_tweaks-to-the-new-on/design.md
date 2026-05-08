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

**Likely root cause now:** The `::highlight(comment-highlight)` rule sets `background-color: #b3d7ff` and `color: #000`, but inside code blocks `react-syntax-highlighter` (Prism, `oneLight` theme) emits `<span>` tokens with **inline `color` styles** (e.g. `color: rgb(...)` for keywords/strings). Inline styles win over the `::highlight()` `color` value in some browser implementations, so the visual contrast disappears inside code — making the highlight look "missing" or "truncated" at the code block boundary even though it is being painted.

**Fix:** Strengthen the `::highlight(comment-highlight)` rule so it visibly applies inside code blocks. Two practical adjustments:

1. Drop the `color` override — keep syntax-token colours intact and rely on `background-color` alone for the highlight effect.
2. If background also fails to render inside `<pre>` tokens (need to verify in-browser), add a more specific `::highlight()` rule scoped to `pre` / `code` descendants, or move the rule to one with no competing inline `background` from the syntax theme.

Verification step in the browser is required before declaring this fixed — open the spec review page, select text spanning a code block, open the comment form, and confirm highlight is visible across all selected text including inside the code.

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
