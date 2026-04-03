# Design: Fix Text Highlighting in Bullet Point Lists

## Root Cause

`applyHighlight()` in `DesignReviewContent.tsx` (line 800) uses:

```typescript
const fragment = range.extractContents(); // removes content from DOM
mark.appendChild(fragment);
range.insertNode(mark);                   // reinserts <mark> at old range position
```

`extractContents()` **removes** the selected nodes from the DOM. When the selection is inside a `<li>`, the text is ripped out of the list item. Then `insertNode(mark)` places the `<mark>` outside the `<li>` context (or at an invalid position within it). The browser "repairs" this malformed DOM by auto-creating a new `<li>` to contain the orphaned `<mark>`, producing the extra list item. The `color: "#000"` in the highlight CSS (line 1012) makes the text appear black even when the blue background is applied — on a light background inside a list this makes it look unhighlighted.

## Fix

Replace `extractContents()` + `insertNode()` with `surroundContents()`:

```typescript
const applyHighlight = (range: Range) => {
  try {
    const mark = document.createElement("mark");
    mark.className = "comment-highlight";
    range.surroundContents(mark);  // wraps in-place, no extraction
    highlightMarkRef.current = mark;
  } catch {
    // surroundContents throws if the range partially spans non-Text nodes
    // (e.g. cross-element selection). In that case, skip the visual highlight
    // but the comment form still opens — acceptable graceful degradation.
    highlightMarkRef.current = null;
  }
};
```

`surroundContents()` wraps the selected content in the provided element **in place**, without extracting and reinserting nodes. This preserves the surrounding DOM structure including `<ul>`/`<ol>`/`<li>` hierarchy.

### Limitation

`surroundContents()` throws a `DOMException` if the range boundary partially intersects an element node (e.g. the selection starts mid-paragraph and ends mid-list-item). This is an edge case — single-element list item selections work fine. The catch block already handles this gracefully by skipping the visual mark while still opening the comment form.

### `removeHighlight()` unchanged

The existing `removeHighlight()` using `mark.replaceWith(...mark.childNodes)` is correct and doesn't need to change.

## Pattern Note

`DesignReviewContent.tsx` uses direct DOM manipulation for text highlighting because ReactMarkdown renders into real DOM nodes and doesn't expose a React-friendly selection API. The `surroundContents()` approach is the standard DOM Range API method for this use case — using `extractContents()` was the wrong choice here.
