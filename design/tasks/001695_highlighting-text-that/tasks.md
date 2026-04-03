# Implementation Tasks

- [ ] In `DesignReviewContent.tsx`, replace `highlightMarkRef` with `savedHighlightRangeRef: MutableRefObject<Range | null>`
- [ ] Rewrite `applyHighlight()` to use `CSS.highlights.set("comment-highlight", new Highlight(range))` instead of DOM manipulation
- [ ] Rewrite `removeHighlight()` to use `CSS.highlights.delete("comment-highlight")`
- [ ] Update GlobalStyles: change `.comment-highlight` selector to `::highlight(comment-highlight)`
- [ ] Line 1256: wrap `removeHighlight()` in condition `if (!showCommentForm)` so highlight persists while typing comment
- [ ] Test: select text in a bullet list — confirm highlight appears, no extra list items, no console errors
- [ ] Test: click into comment text field — confirm highlight remains visible
- [ ] Test: submit comment — confirm highlight clears and list structure is intact
- [ ] Test: cancel/close comment form — confirm highlight clears
