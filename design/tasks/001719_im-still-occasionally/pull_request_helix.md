# Fix link colors and stale text highlight in spec review

## Summary
Two UI bugs in the spec review markdown rendering:
1. Links used browser-default colors (dark blue/purple) which are unreadable on dark backgrounds
2. Text highlight persisted when selecting new text while comment form was open

## Changes
- Add `& a` styles (teal `#00D5FF`, hover underline, visited override) to markdown body in `DesignReviewContent.tsx` and `DesignDocPage.tsx`
- In `handleTextSelection`, call `removeHighlight()` before applying new selection, and call `applyHighlight()` directly (the useEffect doesn't re-fire when `showCommentForm` is already true)
- Preserve the `onMouseDown` guard to avoid clearing highlight prematurely while user is typing a comment
