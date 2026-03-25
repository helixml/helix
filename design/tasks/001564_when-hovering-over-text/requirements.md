# Requirements

## Feature 1: Floating "Add Comment" Button on Hover

### User Story
As a spec reviewer, when I hover over text in the spec view, I want to see a floating "Add Comment" button so I can quickly start a comment without needing to select text first.

### Acceptance Criteria
- A small floating "Add Comment" button (or icon) appears when the user hovers over any paragraph/text block in the spec document.
- The button is positioned near the hovered text (e.g., top-right of the paragraph or near the cursor).
- Clicking the button opens the comment form (same as the existing selection-based flow), but without requiring a prior text selection.
- The button disappears when the cursor leaves the text area.
- The button does not obscure normal reading or interfere with text selection.
- Works on both wide and narrow viewports.

---

## Bug Fix: Text Highlight Disappears When Comment Dialogue Opens

### Bug Description
When a user selects/highlights text in the spec view and the "Add Comment" form appears, the native browser text selection highlight is cleared, leaving no visual indication of which text was selected.

### Acceptance Criteria
- After the comment form opens, the originally selected text remains visually highlighted in the document.
- The highlight persists while the comment form is open (both before and after submission).
- The highlight is removed when the comment is submitted, cancelled, or the form is closed.
- Behaviour is correct on Chrome, Firefox, and Safari.
