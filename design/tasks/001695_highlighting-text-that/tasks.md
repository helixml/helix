# Implementation Tasks

- [ ] In `DesignReviewContent.tsx`, replace `highlightMarkRef` with `savedHighlightRangeRef: MutableRefObject<Range | null>`
- [ ] Rewrite `applyHighlight()` to use `CSS.highlights.set("comment-highlight", new Highlight(range))` instead of DOM manipulation
- [ ] Rewrite `removeHighlight()` to use `CSS.highlights.delete("comment-highlight")`
- [ ] Update GlobalStyles: change `.comment-highlight` selector to `::highlight(comment-highlight)`
- [ ] Test: select text in a bullet list — confirm highlight appears, no extra list items, no console errors
- [ ] Test: select text across multiple list items — confirm graceful degradation (no crash)
- [ ] Test: submit comment — confirm highlight clears and list structure is intact
