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

**Root cause:** `applyHighlight` uses `range.extractContents()` + `range.insertNode(mark)`, which physically moves DOM nodes into a single wrapping `<mark>`. When the range spans a SyntaxHighlighter code block, this disrupts the nested `<span>` structure that SyntaxHighlighter created, causing truncation or breakage.

**Fix:** Replace the single-mark approach with a text-node-walking approach. Walk all text nodes intersecting the range and wrap each selected portion in its own `<mark>`. This leaves the surrounding element structure (including SyntaxHighlighter's spans) intact.

- Change `highlightMarkRef` from `useRef<HTMLElement | null>` to `useRef<HTMLElement[]>` (array of marks)
- New `applyHighlight`: use `document.createTreeWalker(range.commonAncestorContainer, NodeFilter.SHOW_TEXT)`, iterate text nodes, check `range.intersectsNode`, create a clamped sub-range for each, and call `surroundContents(mark)` (works reliably on pure-text ranges)
- New `removeHighlight`: iterate `highlightMarksRef.current`, call `mark.replaceWith(...mark.childNodes)` on each, then reset the array

**CSS consideration:** SyntaxHighlighter tokens use `color` on `<span>` elements. The `.comment-highlight` mark only sets `background-color`, so syntax colors are preserved. If needed, the highlight CSS can use higher specificity to ensure visibility.

**Pattern note:** This codebase uses React MUI with all styling inline via `sx` props and `GlobalStyles`. The `.comment-highlight` class is defined via `GlobalStyles` at line ~1028.
