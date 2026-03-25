# Design

## Context

The spec view is implemented in `frontend/src/components/spec-tasks/DesignReviewContent.tsx` (~1427 lines). It renders markdown documents via `ReactMarkdown` inside a `Box` with a `markdownRef` (the inner markdown container) nested inside a scrollable `Box` with a `documentRef` (the outer document panel).

Text selection is detected via `onMouseUp`/`onTouchEnd` → `handleTextSelection()` (line 777) which calls `window.getSelection()`, validates the selection is within `markdownRef.current`, calculates a `yPosition` relative to `documentRef.current`, then sets `selectedText`, `commentFormPosition`, and `showCommentForm(true)`.

The `InlineCommentForm` (`InlineCommentForm.tsx`) is absolutely positioned to the right (`left: 670px`) on wide viewports (>1000px) and as a fixed bottom sheet on narrow viewports. It has `zIndex: 20`. It auto-focuses its TextField on render, which is the proximate cause of the selection-loss bug.

Viewport width check: `isNarrowViewport = containerWidth > 0 && containerWidth < 1000` (approximate — check exact usage in `DesignReviewContent`).

---

## Feature 1: Floating "Add Comment" Button on Hover

### Approach

Track paragraph-level hover state and render a small floating `IconButton` adjacent to the hovered element.

**Implementation steps:**

1. **Hover detection**: Add `onMouseMove` to the markdown container. On each move, use `document.elementFromPoint()` or walk up from `event.target` to find the nearest block-level element (paragraph, heading, list item) within `markdownRef.current`. Store it in a `hoveredElement` ref.

2. **Button positioning**: Calculate the top-right position of the hovered element's bounding rect relative to `documentRef.current`. Store `{x, y}` in state (`hoverButtonPosition`).

3. **Render**: When `hoveredElement` is set (and no comment form is open), render a small MUI `Tooltip`-wrapped `IconButton` (using `AddCommentIcon` or `ChatBubbleOutlineIcon` from `@mui/icons-material`) absolutely positioned in the document container.

4. **On click**: Call `handleTextSelection()` with the full text of the hovered element as the `selectedText`, or focus a textarea in the comment form with no pre-selected text. Set `showCommentForm(true)`.

5. **Hide on leave**: Add `onMouseLeave` to the markdown container to clear `hoveredElement` and `hoverButtonPosition`.

**Positioning code sketch:**
```tsx
const rect = hoveredElement.getBoundingClientRect();
const containerRect = documentRef.current.getBoundingClientRect();
const y = rect.top - containerRect.top + documentRef.current.scrollTop;
const x = rect.right - containerRect.left; // pin to right edge of text
```

**Wide viewport**: Button positioned absolutely at `{x, y}` relative to the document container.
**Narrow viewport**: Hide the button (users on mobile use touch selection).

---

## Bug Fix: Text Highlight Disappears on Comment Form Open

### Root Cause

When `setShowCommentForm(true)` triggers a React re-render and `InlineCommentForm` mounts, its `TextField` auto-focuses (MUI default). This focus shift causes the browser to immediately clear the native text selection. The highlight disappears even though `selectedText` state still holds the string.

### Fix: Persist Highlight via DOM `<mark>` Injection

The native selection is ephemeral and tied to focus. Replace it with a persistent DOM highlight using a `<mark>` element.

**Flow:**

1. In `handleTextSelection` (line 777), after capturing `text` and calculating `rect`, save `range.cloneRange()` into a `savedRangeRef` **before** calling `setShowCommentForm(true)`.

2. In a `useEffect` that fires when `showCommentForm` becomes `true`, call `applyHighlight(savedRangeRef.current)`:
   - Create `<mark class="comment-highlight">`
   - Use `range.extractContents()` → append to mark → `range.insertNode(mark)` (not `surroundContents` — it throws on cross-element ranges)
   - Store the mark element in `highlightMarkRef`

3. On cancel, submit success, or Escape: call `removeHighlight()` which does `mark.replaceWith(...mark.childNodes)` and clears `highlightMarkRef`.

**CSS (add via MUI `GlobalStyles` inside `DesignReviewContent`):**
```css
.comment-highlight {
  background-color: #b3d7ff;
  color: #000;
  border-radius: 2px;
}
```

**Caveats:**
- Wrap `applyHighlight` in try/catch — fall back to no highlight if DOM manipulation fails
- The `<mark>` exists outside React's virtual DOM; cleanup must be imperative (ref-based), not declarative
- Cleanup (`removeHighlight`) must be called everywhere `selectedText` is cleared: cancel handler, submit success, and Escape key handler (search for `setShowCommentForm(false)` and `setSelectedText("")` call sites)

---

## Key Files to Modify

- `frontend/src/components/spec-tasks/DesignReviewContent.tsx` — all changes live here (line refs: `handleTextSelection` ~777, hover styles ~1260, form render ~1344)
- `frontend/src/components/spec-tasks/InlineCommentForm.tsx` — no changes needed
- Add `.comment-highlight` CSS class via MUI `GlobalStyles` component inside `DesignReviewContent`

## Patterns Found in Codebase

- Hover button should use MUI `IconButton` + `Tooltip` pattern — already used throughout the file (e.g., `CommentIcon` tooltip at line ~1158)
- `documentRef` is the scrollable outer container; `markdownRef` is the inner markdown Box — use `documentRef` for scroll offset math, `markdownRef` for element containment checks
- `isNarrowViewport` prop is already threaded through to `InlineCommentForm` — reuse this for hiding the hover button on mobile
- Block elements to target for hover: `p, li, h1, h2, h3, h4, blockquote, pre` — matches the existing hover style selectors at ~line 1260
