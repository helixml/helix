# Implementation Tasks

## Bug 1: Link colors
- [ ] Add `"& a"` styles (color, hover, visited) to the `"& .markdown-body"` sx block in `frontend/src/components/spec-tasks/DesignReviewContent.tsx` (~line 1362)
- [ ] Add `'& a'` styles to the `Box` sx block in `frontend/src/pages/DesignDocPage.tsx` (~line 222)

## Bug 2: Stale highlight
- [ ] In `handleTextSelection` (~line 822), call `removeHighlight()` at the start of `processSelection` and call `applyHighlight()` directly after setting up the new selection
- [ ] Remove the `if (!showCommentForm)` guard from the `onMouseDown` handler (~line 1257) so clicking always clears stale highlights

## Verification
- [ ] Verify links render in teal on dark background in spec review
- [ ] Verify selecting new text clears previous highlight (with and without comment form open)
- [ ] Verify clicking off clears highlight
