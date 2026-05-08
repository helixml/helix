# Implementation Tasks

All changes in `frontend/src/components/spec-tasks/DesignReviewContent.tsx`.

- [x] In the hover button `onClick`, create a `Range` over `hoveredElementRef.current` and assign it to `savedRangeRef.current` before setting `showCommentForm = true`, so the existing `useEffect` applies the pseudo-highlight
- [x] Add `onMouseMove` to the outer scroll container (the Box with `onMouseLeave`) that clears `hoverButtonPosition` when the cursor x-position exceeds the button's right edge (`containerWidth/2 + 432px`)
- [ ] Adjust the `::highlight(comment-highlight)` GlobalStyles rule (line ~1116) so the highlight is visible across code blocks: drop the conflicting `color` override and verify in-browser that `background-color` paints inside `<pre>` tokens; add a more specific selector for `pre`/`code` descendants if needed
- [x] At the top of the inner Box's `onMouseMove` handler, check if `e.target` is contained within any `commentRefs.current` entry; if so, clear `hoverButtonPosition` and `hoveredElementRef.current` and return early
- [ ] Run `cd frontend && yarn build` to verify no TypeScript errors before committing
- [ ] Manual browser test in the inner Helix: register/login, create a spec task, navigate to its design review, verify (a) clicking hover button highlights the paragraph, (b) moving cursor right past the button hides it, (c) selection across code block highlights correctly, (d) hovering over a comment panel hides the button
