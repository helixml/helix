# Requirements: Comment Box Overlap Fix

## Problem

When reviewing a spec, inline comment bubbles (`InlineCommentBubble`) and the new comment form (`InlineCommentForm`) are positioned at `left: "670px"` within the spec document's containing box (which is `maxWidth: "800px"`). This means the comment panel starts 130px inside the right edge of the spec document, causing visible overlap.

The overlap happens on wide viewports (> 1000px) where there is ample horizontal space to place the comment panel fully to the right of the spec.

## User Story

As a reviewer reading a spec, I want inline comment panels to appear to the **right** of the document without overlapping it, so I can read the spec content and the comment side by side without visual interference.

## Acceptance Criteria

- [ ] On wide viewports (> 1000px), the inline comment bubble/form starts at or after the right edge of the spec document (800px), with a small gap (e.g., ≥ 10px)
- [ ] No visible overlap between the spec document text area and any comment panel on wide viewports
- [ ] Narrow viewport behavior (≤ 1000px) is unchanged — comments render below the document or as a bottom sheet
- [ ] The fix applies to both `InlineCommentBubble` and `InlineCommentForm`
