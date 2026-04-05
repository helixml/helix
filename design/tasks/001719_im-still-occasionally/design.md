# Design: Fix Link Colors & Stale Highlight in Spec Review

## Bug 1: Dark link colors on dark background

### Problem

Links rendered by `ReactMarkdown` use browser-default colors (dark blue `#0000FF` unvisited, dark purple `#800080` visited). Nearly invisible on dark grey backgrounds.

### Root Cause

Both `DesignReviewContent.tsx` (~line 1287) and `DesignDocPage.tsx` (~line 174) define `sx` styles for headings, paragraphs, lists, code, blockquotes — but completely omit `& a` anchor tag styles. No global anchor override exists in the MUI theme either.

### Fix

Add `"& a"` styles to both markdown containers:

```tsx
"& a": {
  color: "#00D5FF",
  textDecoration: "none",
  "&:hover": {
    textDecoration: "underline",
  },
  "&:visited": {
    color: "#00D5FF",
  },
},
```

Uses teal `#00D5FF` (the theme's `tealRoot` accent) — high contrast on dark backgrounds, consistent with existing accent usage (`rgba(0,229,255,0.13)` hover highlights in menus).

---

## Bug 2: Stale text highlight persists after re-selection

### Problem

When you highlight text in the spec reviewer (which opens the comment form), then highlight different text, the first highlight remains visually — you end up with two highlighted regions but only one is the real selection.

### Root Cause

The component uses the CSS Highlight API (`CSS.highlights.set("comment-highlight", ...)`) to preserve the visual selection when the comment form's TextField steals focus (line 210-217).

The clearing logic in `onMouseDown` (line 1257) is:
```tsx
onMouseDown={() => { if (!showCommentForm) removeHighlight(); }}
```

This guard means: when the comment form IS open, `mouseDown` does NOT clear the old highlight. The intent was to keep the highlight visible while the user types a comment. But when the user selects new text while the form is open, `handleTextSelection` (on `mouseUp`) creates a new selection and the `useEffect` on `showCommentForm` applies a new highlight — without ever clearing the old one. `CSS.highlights.delete("comment-highlight")` in `applyHighlight` does run (line 803), but since the Highlight API uses a single named highlight, the old range should be replaced. However, the issue is that `applyHighlight` is called via the `useEffect` which only fires when `showCommentForm` *changes*. Since it's already `true`, the effect doesn't re-run for subsequent selections.

So the flow is:
1. Select "dog" → `mouseUp` → `handleTextSelection` → `setShowCommentForm(true)` → effect fires → `applyHighlight(range)` ✓
2. Select "street" → `mouseDown` skipped (form open) → `mouseUp` → `handleTextSelection` → `setShowCommentForm(true)` (already true, no state change) → effect does NOT re-fire → old "dog" highlight stays, new native selection shows "street"

### Fix

Call `removeHighlight()` at the start of `handleTextSelection`, before processing the new selection. This clears any existing CSS Highlight API highlight before the new one gets applied. The `applyHighlight` in the `useEffect` won't re-fire (since `showCommentForm` didn't change), so we also need to call `applyHighlight` directly from `handleTextSelection` when the form is already open.

In `handleTextSelection` (~line 822), add at the top of `processSelection`:
```tsx
const processSelection = () => {
  removeHighlight(); // Clear stale highlight before processing new selection
  const selection = window.getSelection();
  // ... existing code ...
  if (containerRect) {
    // ... existing position code ...
    savedRangeRef.current = range.cloneRange();
    setSelectedText(text);
    setCommentFormPosition({ x: 0, y: yPosition });
    setShowCommentForm(true);
    // Apply highlight immediately (useEffect won't fire if form was already open)
    applyHighlight(range.cloneRange());
  }
};
```

Also remove the `if (!showCommentForm)` guard from the `onMouseDown` handler so that clicking (without selecting) always clears stale highlights:
```tsx
onMouseDown={() => { removeHighlight(); }}
```

The `useEffect` on `showCommentForm` (line 212-217) still handles the initial case (form going from closed → open), so it remains unchanged.
