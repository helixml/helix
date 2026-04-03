# Fix text highlighting in bullet point lists on spec review page

## Summary

Selecting text in bullet point lists on the spec review page was broken - it caused spurious list items to appear, React DOM reconciliation errors (`removeChild`/`insertBefore` errors), and the highlight wasn't visually applied. This was caused by using `extractContents()` and `insertNode()` to wrap selected text in a `<mark>` element, which corrupts the list DOM structure and desyncs React's virtual DOM.

The fix replaces direct DOM manipulation with the CSS Custom Highlight API, which creates visual highlights without modifying the DOM tree at all.

## Changes

- Replace `highlightMarkRef` (HTMLElement) with `savedHighlightRangeRef` (Range) since we no longer store DOM elements
- Rewrite `applyHighlight()` to use `CSS.highlights.set("comment-highlight", new Highlight(range))`
- Rewrite `removeHighlight()` to use `CSS.highlights.delete("comment-highlight")`
- Update GlobalStyles selector from `.comment-highlight` to `::highlight(comment-highlight)`
- Preserve highlight while typing comment by only calling `removeHighlight()` on mousedown when comment form is not open

## Testing

- Tested selecting text in bullet points - highlight appears correctly
- No console errors (previously showed `removeChild`/`insertBefore` DOM errors)
- List structure preserved (no spurious list items created)
- Highlight persists while clicking in comment text field
- Highlight clears on cancel or submit
