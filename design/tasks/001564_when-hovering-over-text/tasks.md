# Implementation Tasks

## Bug Fix: Preserve Text Highlight When Comment Form Opens

- [ ] In `handleTextSelection`, save `range.cloneRange()` to a `savedRangeRef` before calling `setShowCommentForm(true)`
- [ ] Add a `highlightMarkRef` ref to store the injected `<mark>` element
- [ ] Add a `applyHighlight(range)` helper that wraps the range in a `<mark class="comment-highlight">` using `extractContents`/`insertNode` (with try/catch fallback)
- [ ] Add a `removeHighlight()` helper that replaces the `<mark>` with its child nodes and clears `highlightMarkRef`
- [ ] Add a `useEffect` on `showCommentForm` to call `applyHighlight` when it becomes `true`
- [ ] Call `removeHighlight()` in the cancel handler, submit success handler, and Escape key handler (wherever `selectedText` is cleared)
- [ ] Add `.comment-highlight` CSS styles via MUI `GlobalStyles` in `DesignReviewContent` (`background: #b3d7ff; color: #000; border-radius: 2px`)

## Feature: Floating "Add Comment" Button on Hover

- [ ] Add state: `hoverButtonPosition: { x: number; y: number } | null` and a `hoveredElementRef` ref in `DesignReviewContent`
- [ ] Add `onMouseMove` handler on the markdown container that walks up from `event.target` to find the nearest block element within `markdownRef.current`, calculates its bounding position relative to `documentRef.current`, and sets `hoverButtonPosition`
- [ ] Add `onMouseLeave` handler on the markdown container to clear `hoverButtonPosition`
- [ ] Hide hover button when `showCommentForm` is true (avoid overlapping UI)
- [ ] Render a floating `IconButton` (with `ChatBubbleOutlineIcon` or similar) absolutely positioned at `hoverButtonPosition` inside the document container; wrap in a `Tooltip` with label "Add comment"
- [ ] Clicking the button sets `selectedText` to the hovered element's `innerText` (trimmed), sets `commentFormPosition` to the hover position, and calls `setShowCommentForm(true)`
- [ ] Hide the floating button on narrow viewports (≤1000px) to avoid clutter on mobile
- [ ] Verify the button does not flicker when moving the mouse within the same block element (debounce or compare element identity before updating state)
