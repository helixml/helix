# Implementation Tasks

## Bug 1: Link colors
- [x] Add `"& a"` styles (color, hover, visited) to the `"& .markdown-body"` sx block in `frontend/src/components/spec-tasks/DesignReviewContent.tsx` (~line 1362)
- [x] Add `'& a'` styles to the `Box` sx block in `frontend/src/pages/DesignDocPage.tsx` (~line 222)

## Bug 2: Stale highlight
- [~] In `handleTextSelection` (~line 822), inside `processSelection`, call `removeHighlight()` only after confirming a valid new selection exists (not on empty clicks), then call `applyHighlight()` directly with the new range
- [ ] Do NOT change the `onMouseDown` guard — it must remain `if (!showCommentForm) removeHighlight()` to prevent premature clearing while the user is typing a comment

## Verification
- [ ] Verify links render in teal on dark background in spec review
- [ ] Verify selecting new text clears previous highlight (with and without comment form open)
- [ ] Verify clicking off clears highlight
