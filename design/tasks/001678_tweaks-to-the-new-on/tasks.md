# Implementation Tasks

All changes in `frontend/src/components/spec-tasks/DesignReviewContent.tsx`.

- [ ] In the hover button `onClick`, create a `Range` over `hoveredElementRef.current` and assign it to `savedRangeRef.current` before setting `showCommentForm = true`, so the existing `useEffect` applies the pseudo-highlight
- [ ] Add `onMouseMove` to the outer scroll container (the Box with `onMouseLeave`) that clears `hoverButtonPosition` when the cursor x-position exceeds the button's right edge (`containerWidth/2 + 432px`)
- [ ] Change `highlightMarkRef` from `useRef<HTMLElement | null>` to `useRef<HTMLElement[]>` to support multiple mark elements
- [ ] Rewrite `applyHighlight` to walk text nodes within the range using `createTreeWalker` and wrap each intersecting text node in its own `<mark class="comment-highlight">` via `surroundContents`, leaving surrounding element structure intact
- [ ] Update `removeHighlight` to iterate the marks array, unwrap each mark's children, and reset the array
- [ ] Run `cd frontend && yarn build` to verify no TypeScript errors before committing
