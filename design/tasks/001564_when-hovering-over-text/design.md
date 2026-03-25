# Design

## Context

The spec view is implemented in `DesignReviewContent.tsx` (~1427 lines). It renders markdown documents via `ReactMarkdown` inside a `Box` with a `markdownRef`. Text selection is detected via `onMouseUp`/`onTouchEnd` → `handleTextSelection()` which calls `window.getSelection()` and sets `showCommentForm(true)`.

The comment form (`InlineCommentForm`) is absolutely positioned to the right of the document on wide viewports and as a bottom sheet on narrow viewports.

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

When `setShowCommentForm(true)` triggers a React re-render, the new DOM elements cause the browser to clear its native text selection. The blue highlight disappears even though `selectedText` state still holds the string.

### Fix: Manual Highlight via DOM Range

Before clearing/losing the selection, save the `Range` object. After the form renders, apply a synthetic highlight using a `<mark>` wrapper or the CSS Custom Highlight API.

**Recommended approach — wrap with `<mark>`:**

1. In `handleTextSelection`, after capturing `text` and `rect`, also save `range.cloneRange()` into a ref (`savedRangeRef`).

2. After `setShowCommentForm(true)` (in a `useEffect` that watches `showCommentForm`), call a helper `applyHighlight(savedRangeRef.current)`:
   - Create a `<mark>` element with a CSS class (e.g., `comment-highlight`) styled with `background: #b3d7ff; color: #000;`
   - Use `range.surroundContents(mark)` — but this can fail if the range spans multiple elements, so use `range.extractContents()` + `mark.appendChild(fragment)` + `range.insertNode(mark)` instead.

3. When the comment form is cancelled or submitted, remove the `<mark>` element and restore the original text nodes (`mark.replaceWith(...mark.childNodes)`). Store the mark element in a ref (`highlightMarkRef`).

**Alternative — CSS Custom Highlight API** (Chrome 105+, Firefox 119+): Use `CSS.highlights` for zero-DOM-mutation highlighting. This is cleaner but has less browser support. Given the primary users are likely on modern Chromium, this is viable.

**Chosen approach:** `<mark>` wrapping — wider browser support, straightforward implementation, consistent with existing ReactMarkdown output manipulation patterns used for comment position calculation.

**CSS:**
```css
.comment-highlight {
  background-color: #b3d7ff;
  color: #000;
  border-radius: 2px;
}
```

Add the class to the global stylesheet or via a MUI `GlobalStyles` component in `DesignReviewContent`.

### Caveats
- `surroundContents` throws if the range boundary is in the middle of a tag. Use `extractContents`/`insertNode` pattern instead, and wrap in try/catch with fallback to no highlight.
- The `<mark>` is injected into ReactMarkdown-rendered DOM. React doesn't know about it, so cleanup must be imperative (ref-based), not declarative.
- Call cleanup (`removeHighlight`) in the same places where `selectedText` is cleared: cancel, submit success, Escape key handler.

---

## Key Files to Modify

- `frontend/src/components/spec-tasks/DesignReviewContent.tsx` — all changes live here
- `frontend/src/components/spec-tasks/InlineCommentForm.tsx` — no changes needed
- Possibly add a small CSS class via MUI `GlobalStyles` within `DesignReviewContent`
