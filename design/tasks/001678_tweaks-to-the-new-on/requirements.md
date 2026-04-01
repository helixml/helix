# Requirements: Tweaks to On-Hover Add Comment Button

## Context

`DesignReviewContent.tsx` has an on-hover "add comment" button that appears to the right of block-level elements (P, H1-H4, LI, BLOCKQUOTE, PRE). When clicked, it opens the comment form. There is also a "pseudo-highlight" mechanism that wraps selected text in `<mark class="comment-highlight">` to preserve the visual highlight while the comment form is open (since form focus clears the native browser selection).

## User Stories

### 1. Highlight text on hover-button click

**As a** reviewer,
**When** I click the on-hover add comment button,
**I want** the paragraph/element text to be highlighted (with the same blue pseudo-highlight used when selecting text manually),
**So that** it's clear which block of text my comment is attached to.

**Acceptance Criteria:**
- Clicking the hover button applies a visual highlight to the hovered element (same `#b3d7ff` style as the manual-selection highlight)
- The highlight persists while the comment form is open
- The highlight is removed when the comment form is dismissed or submitted

### 2. Hide hover button when cursor moves past its right edge

**As a** reviewer,
**When** I move my mouse cursor to the right of the comment button (past its right edge),
**I want** the button to disappear,
**So that** the button doesn't linger after I've moved on.

**Acceptance Criteria:**
- The button disappears when the cursor moves to the right past the button's right edge
- The button does NOT disappear when the cursor is directly over the button (clicking must still work)
- The button does NOT disappear when the cursor moves back left (toward the document text)
- No regression: button still appears/disappears correctly on paragraph hover and mouse-leave

### 3. Fix pseudo-highlight truncation when selection spans a code block

**As a** reviewer,
**When** I select text that spans across a code block (e.g., text above and inside/below a code block),
**I want** the pseudo-highlight to correctly cover my full selection including the code block content,
**So that** the highlight is not truncated at the code block boundary.

**Acceptance Criteria:**
- Pseudo-highlight correctly covers text before, inside, and after a code block
- Syntax highlighting inside the code block remains visually correct under the highlight (colors may blend but structure must be intact)
- The highlight is removable without leaving orphaned DOM artifacts

### 4. No hover button when cursor is over a comment panel

**As a** reviewer,
**When** I move my mouse over an existing inline comment panel (`InlineCommentBubble`),
**I want** the add-comment hover button to not appear (or to disappear if already visible),
**So that** comment panels are not obscured by an irrelevant button.

**Acceptance Criteria:**
- Moving the cursor over any `InlineCommentBubble` clears any visible hover button
- The hover button does not re-appear while the cursor remains over the comment panel
- Moving the cursor back to document text resumes normal hover-button behaviour
