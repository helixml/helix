# Add hover "Add comment" button and fix highlight persistence on spec view

## Summary
- Floating "Add Comment" button appears when hovering over any paragraph, heading, or list item in the spec review document
- Bug fix: selected text stays visually highlighted (blue) while the comment form is open

## Changes
- `DesignReviewContent.tsx`: all changes confined to this one file
  - Hover button: `onMouseMove` walks up from the hovered element to find the nearest block element, calculates its position, and renders an `AddCommentIcon` `IconButton` at the right edge of the document column; hidden when form is open or on narrow viewports
  - Highlight bug fix: saves the selection `Range` before opening the form, then injects a `<mark class="comment-highlight">` element after the form mounts (replacing the now-lost native browser selection); `removeHighlight()` is called in all cancel/submit paths (Escape, cancel button, submit success)
  - `GlobalStyles` added for `.comment-highlight { background-color: #b3d7ff; ... }`
