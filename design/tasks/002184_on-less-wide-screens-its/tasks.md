# Implementation Tasks: Make Comment Resolve Button Discoverable on Narrow Screens

- [x] In `DesignReviewContent.tsx`, raise the `isNarrowViewport` breakpoint from `theme.breakpoints.down(1000)` to ~`down(1280)` so the stacked layout engages before the side-positioned bubble overflows (no horizontal scroll); update the explanatory comment accordingly.
- [x] In `InlineCommentBubble.tsx`, replace the bare resolve `IconButton` (CheckCircleIcon) in the comment header with a `Tooltip`-wrapped labeled `Button` ("Resolve", `size="small"`, `color="success"`, `startIcon={<CheckCircleIcon fontSize="small" />}`), keeping the `onResolve(comment.id!)` call.
- [x] Add `Tooltip` to the `@mui/material` import in `InlineCommentBubble.tsx` (also removed now-unused `IconButton`).
- [x] In `CommentLogSidebar.tsx`, apply the same labeled `Button` for the resolve action (only when `!comment.resolved`), keeping the `onResolveComment(comment.id!)` call.
- [x] Add `Button` and `Tooltip` to the `@mui/material` import in `CommentLogSidebar.tsx` (removed now-unused `IconButton`).
- [ ] Verify the comment header layout is intact on wide viewports (Chip left, Resolve right, no overflow).
- [~] Build the frontend (`cd frontend && yarn build`).
- [ ] Test end-to-end in inner Helix: open a spec task review with an inline comment; confirm the "Resolve" label is visible and works, and confirm there is NO horizontal scroll needed to reach the bubble across small, medium (~1000-1280px), and wide window widths.
