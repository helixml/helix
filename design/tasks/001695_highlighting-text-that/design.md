# Design: Fix Text Highlighting in Bullet Point Lists

## Root Cause

Two problems in `applyHighlight()` at `DesignReviewContent.tsx:800`:

**1. DOM structure corruption:**
```typescript
const fragment = range.extractContents(); // removes content from DOM
mark.appendChild(fragment);
range.insertNode(mark);                   // reinserts <mark> at old range position
```
`extractContents()` rips text out of `<li>` elements. The browser "repairs" this by creating new list items.

**2. React/DOM desync (the console errors):**
Direct DOM manipulation (`extractContents`, `insertNode`, `removeChild`) modifies nodes that React's virtual DOM thinks it owns. On the next React reconciliation cycle, React expects nodes in certain positions but they've been moved:
```
NotFoundError: Failed to execute 'removeChild' on 'Node': The node to be removed is not a child of this node
NotFoundError: Failed to execute 'insertBefore' on 'Node': The node before which the new node is to be inserted is not a child of this node
```

**3. Highlight disappears when clicking comment box:**
Line 1256 has `onMouseDown={() => removeHighlight()}` on the document container Box. Any mousedown inside that container clears the highlight — including clicking into the comment TextField.

## Fix: CSS Custom Highlight API

Use the CSS Custom Highlight API instead of DOM manipulation. This creates a visual highlight overlay without modifying the DOM tree at all — no React desync, no list structure corruption.

```typescript
const applyHighlight = (range: Range) => {
  try {
    // Clear any existing highlight
    CSS.highlights.delete("comment-highlight");

    // Create highlight from the range - no DOM modification
    const highlight = new Highlight(range);
    CSS.highlights.set("comment-highlight", highlight);

    // Store range for cleanup (not a DOM node)
    savedHighlightRangeRef.current = range;
  } catch {
    // Fallback: skip visual highlight, comment form still opens
    savedHighlightRangeRef.current = null;
  }
};

const removeHighlight = () => {
  CSS.highlights.delete("comment-highlight");
  savedHighlightRangeRef.current = null;
};
```

Replace the GlobalStyles highlight CSS with:
```typescript
<GlobalStyles styles={{
  "::highlight(comment-highlight)": {
    backgroundColor: "#b3d7ff",
    color: "#000",
  }
}} />
```

### Why this works
- `CSS.highlights` is a registry of named highlights
- `Highlight` objects wrap `Range` objects to style them visually
- The DOM tree is never modified — React's virtual DOM stays in sync
- List structure is preserved because we're not touching DOM nodes

### Browser Support
CSS Custom Highlight API is supported in Chrome 105+, Edge 105+, Safari 17.2+. For older browsers, degrade gracefully (no visual highlight but comment form still works).

### Ref change
Change `highlightMarkRef` from storing a DOM element to storing the Range:
- Old: `highlightMarkRef: MutableRefObject<HTMLElement | null>`
- New: `savedHighlightRangeRef: MutableRefObject<Range | null>`

## Fix: Preserve highlight while typing comment

**Problem:** Line 1256 clears highlight on any mousedown in the document container:
```typescript
onMouseDown={() => removeHighlight()}
```

**Solution:** Only clear highlight if comment form is not open:
```typescript
onMouseDown={() => {
  if (!showCommentForm) {
    removeHighlight();
  }
}}
```

This keeps the highlight visible while the user types their comment. The highlight is cleared:
- When the comment is submitted (line 881)
- When the comment form is cancelled/closed (line 1454)
- When user starts a new text selection (new selection replaces old)

## Alternative Considered

Using `surroundContents()` instead of `extractContents()` — less invasive DOM manipulation but still modifies the DOM tree, which would still cause React reconciliation issues on re-renders. The CSS Highlight API is the correct solution for highlighting in React-rendered content.
