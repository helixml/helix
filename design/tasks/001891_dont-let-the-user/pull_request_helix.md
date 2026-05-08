# Gate spec approval on tab viewing and improve resolve button icon

## Summary
Prevents users from approving a spec design until they've viewed all three tabs (Requirements, Technical Design, Implementation Plan). Also changes the comment resolve button from an ambiguous X icon to a green checkmark.

## Changes
- **ReviewActionFooter**: Disable "Approve Design" button when not all tabs viewed, with tooltip showing which tabs remain
- **DesignReviewContent**: Wire existing `viewedTabs` state to the footer; add content-change detection that invalidates viewed tabs when the agent revises content; add orange dot indicator on unviewed tab labels; refactor tab rendering to use `.map()`
- **InlineCommentBubble**: Replace `CloseIcon` with green `CheckCircleIcon` on resolve button
- **CommentLogSidebar**: Same resolve icon change; remove unused `CloseIcon` import
