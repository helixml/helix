# Requirements

## Feature 1: Floating "Add Comment" Button on Hover

### User Story
As a spec reviewer, when I hover over text in the spec view, I want to see a floating "Add Comment" button so I can quickly start a comment without needing to select text first.

### Acceptance Criteria
- A small floating "Add Comment" button (or icon) appears when the user hovers over any paragraph/text block in the spec document (`p`, `li`, `h1`–`h4`, `blockquote`, code blocks).
- The button is positioned at the top-right edge of the hovered block element, anchored to the text column (not the sidebar).
- Clicking the button opens the InlineCommentForm with the hovered element's full text pre-set as `selectedText` — same as the existing selection-based flow.
- The button disappears when the cursor leaves the markdown content area.
- The button is hidden while the InlineCommentForm is already open.
- The button does not flicker when moving between sibling elements — compare element identity before updating position state.
- Hidden on narrow viewports (≤1000px) where touch/selection is the primary input.

---

## Bug Fix: Text Highlight Disappears When Comment Dialogue Opens

### Bug Description
When a user selects/highlights text in the spec view and the "Add Comment" form appears, the native browser text selection is cleared. This happens because React re-renders (triggered by `setShowCommentForm(true)`) and/or focus shifting to the form's TextField cause the browser to drop the native selection, even though `selectedText` state still holds the string.

### Acceptance Criteria
- After the comment form opens, the originally selected text remains visually highlighted (blue background) in the document.
- The highlight persists while the comment form is open, both before and after the user starts typing their comment.
- The highlight is removed when the comment is submitted, cancelled (including Escape key), or the form is closed.
- Works correctly when the selected range spans multiple inline elements within a paragraph.
- Falls back gracefully (no error, no crash) if the DOM manipulation fails (e.g., cross-element range boundary issues).
