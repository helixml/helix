# Fix comment panel overlapping spec document on wide viewports

## Summary

Inline comment bubbles and the new comment form were positioned at `left: "670px"` within the spec document's 800px-wide container. This caused 130px of overlap with the spec content. Changed to `left: "820px"` so comments appear 20px to the right of the document with no overlap.

## Changes

- `frontend/src/components/spec-tasks/InlineCommentBubble.tsx`: `left: "670px"` → `left: "820px"`
- `frontend/src/components/spec-tasks/InlineCommentForm.tsx`: `left: "670px"` → `left: "820px"`
