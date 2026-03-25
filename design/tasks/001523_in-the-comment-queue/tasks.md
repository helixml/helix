# Implementation Tasks

- [ ] In `DesignReviewContent.tsx`, add `isComplete?: boolean` to the `streamingResponse` state type
- [ ] In `DesignReviewContent.tsx`, replace both `setStreamingResponse(null)` calls (in `session_update` and `interaction_update` completion handlers) with `setStreamingResponse(prev => prev ? { ...prev, isComplete: true } : null)`
- [ ] In `DesignReviewContent.tsx`, update the `isStreaming` computation passed to `InlineCommentBubble` and `CommentLogSidebar` to use `isActiveStream && !comment.agent_response && !streamingResponse?.isComplete`
- [ ] In `CommentLogSidebar.tsx`, update the `isStreaming` local variable to add `&& !streamingResponse?.isComplete` (requires passing `isComplete` through the `StreamingResponse` interface)
- [ ] Verify: after comment 1 completes and comment 2 starts streaming, comment 1 shows its response (collapsed last 4 lines, expandable) with no spinner
- [ ] Verify: comment 2 continues to stream correctly, unaffected by the change
