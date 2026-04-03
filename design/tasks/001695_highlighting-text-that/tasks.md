# Implementation Tasks

- [x] In `DesignReviewContent.tsx`, replace `highlightMarkRef` with `savedHighlightRangeRef: MutableRefObject<Range | null>`
- [x] Rewrite `applyHighlight()` to use `CSS.highlights.set("comment-highlight", new Highlight(range))` instead of DOM manipulation
- [x] Rewrite `removeHighlight()` to use `CSS.highlights.delete("comment-highlight")`
- [x] Update GlobalStyles: change `.comment-highlight` selector to `::highlight(comment-highlight)`
- [x] Line 1256: wrap `removeHighlight()` in condition `if (!showCommentForm)` so highlight persists while typing comment
- [x] Test: select text in a bullet list — confirm highlight appears, no extra list items, no console errors
- [x] Test: click into comment text field — confirm highlight remains visible
- [x] Test: submit comment — confirm highlight clears and list structure is intact
- [x] Test: cancel/close comment form — confirm highlight clears
