# Requirements: Prevent New Comment Form From Overlapping Existing Comment Bubbles on Spec Review Page

## Background

On the spec review page (the `DesignReviewContent` view used in the spec-task
flow), reviewers select text in a markdown document and an inline "Add
Comment" form opens to the right of the document. Once a comment has been
submitted, it stays anchored to its quoted text as an `InlineCommentBubble`
in the right-hand column.

There is already a collision-avoidance algorithm for the bubbles
(`DesignReviewContent.tsx:706-805`): bubbles whose anchor positions are
close together get pushed down by `minGap = 10px` so they stack neatly
instead of overlapping.

The bug is that this stacking math runs over `inlineComments` only — it
does **not** include the open `InlineCommentForm`. So when the user opens
a new comment near an existing one, the form is positioned at the raw
selection Y while the bubble underneath stays where it is, and the two
visually overlap in the 300-px right-hand column.

## User Story

As a reviewer on the spec review page, when I add a comment and then
select new text near the existing comment to open another one, I want the
new comment form to appear in a clean, readable position — not overlapping
the existing comment bubble.

## Acceptance Criteria

1. With at least one existing comment bubble visible, selecting new text
   whose anchor Y is within the vertical bounds of an existing bubble
   opens the new comment form **without** any visual overlap with that
   bubble.
2. The new comment form is pushed down (or the bubble repositions) such
   that there is at least the existing `minGap` (10px) of clear space
   between the form and any neighbouring bubble.
3. The existing single-bubble and multi-bubble stacking behaviour
   continues to work unchanged when no form is open.
4. Cancelling or submitting the form returns bubbles to their normal
   stacked positions (no leftover gap).
5. The fix works on wide viewports (where positioning is absolute in the
   right column). Narrow-viewport behaviour — where bubbles render
   inline below the document and the form is a bottom-sheet — is already
   non-overlapping and must remain unchanged.
6. No regressions to: highlight preservation, auto-scroll-into-view of
   the form, Cmd/Ctrl+Enter submit, the comment-resolved → bubble-removed
   re-stack cycle.

## Out of Scope

- Reworking the hard-coded `left: 820px` column position.
- Changing the right column to a draggable / collapsible panel.
- Any backend or API changes — this is a pure frontend layout fix.
