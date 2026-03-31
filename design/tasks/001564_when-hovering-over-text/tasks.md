# Implementation Tasks

## Bug Fix: Preserve Text Highlight When Comment Form Opens

All changes in `frontend/src/components/spec-tasks/DesignReviewContent.tsx`

- [x] Add `savedRangeRef = useRef<Range | null>(null)` and `highlightMarkRef = useRef<HTMLElement | null>(null)`
- [x] In `handleTextSelection` (line ~806), before `setShowCommentForm(true)`, save `savedRangeRef.current = range.cloneRange()`
- [x] Add `applyHighlight(range: Range)` helper: create `<mark class="comment-highlight">`, use `range.extractContents()` + `mark.appendChild(fragment)` + `range.insertNode(mark)`, store in `highlightMarkRef`; wrap in try/catch (silent fallback)
- [x] Add `removeHighlight()` helper: if `highlightMarkRef.current`, call `mark.replaceWith(...mark.childNodes)` and null the ref
- [x] Add `useEffect(() => { if (showCommentForm && savedRangeRef.current) applyHighlight(savedRangeRef.current) }, [showCommentForm])`
- [x] Call `removeHighlight()` in all cancel paths (cancel button handler, Escape key handler, submit success) — search for `setShowCommentForm(false)` and `setSelectedText("")` call sites
- [x] Add MUI `GlobalStyles` inside `DesignReviewContent` with `.comment-highlight { background-color: #b3d7ff; color: #000; border-radius: 2px; }`

## Feature: Floating "Add Comment" Button on Hover

All changes in `frontend/src/components/spec-tasks/DesignReviewContent.tsx`

- [x] Add state: `hoverButtonPosition: { x: number; y: number; elementText: string } | null` (null = hidden); add `hoveredElementRef = useRef<Element | null>(null)`
- [x] Add `onMouseMove` handler on the markdown content Box (alongside existing `onMouseUp`): walk up from `event.target` to find the nearest block element (`P, LI, H1–H4, BLOCKQUOTE, PRE`) that is a descendant of `markdownRef.current`; if different from `hoveredElementRef.current`, update ref and calculate position using `element.getBoundingClientRect()` minus `documentRef.current.getBoundingClientRect()` plus `documentRef.current.scrollTop`; set `hoverButtonPosition`
- [x] Add `onMouseLeave` handler on the same Box to clear `hoverButtonPosition` and `hoveredElementRef.current`
- [x] Render floating `IconButton` (use `AddCommentIcon` or `ChatBubbleOutlineIcon` from `@mui/icons-material`) inside the document container `Box`, conditionally when `hoverButtonPosition !== null && !showCommentForm && !isNarrowViewport`; position absolute at `{top: hoverButtonPosition.y, left: hoverButtonPosition.x}`; wrap in `<Tooltip title="Add comment">`
- [x] On button click: set `selectedText = hoverButtonPosition.elementText.trim()`, set `commentFormPosition = { x: 0, y: hoverButtonPosition.y }`, call `setShowCommentForm(true)`, clear `hoverButtonPosition`
- [x] Verify no flickering: only call `setHoverButtonPosition` when the element reference actually changes (compare to `hoveredElementRef.current`)
