# Implementation Tasks

## Code Changes (implemented and verified)

- [x] Add `isComplete?: boolean` to `streamingResponse` state type in `DesignReviewContent.tsx`
- [x] In `DesignReviewContent.tsx` `session_update` completion handler (~line 475): replace `setStreamingResponse(null)` with `setStreamingResponse(prev => prev ? { ...prev, isComplete: true } : null)`
- [x] In `DesignReviewContent.tsx` `interaction_update` completion handler (~line 532): same replacement
- [x] Add `isStreamingComplete?: boolean` prop to `InlineCommentBubble` interface and destructuring
- [x] In `InlineCommentBubble.tsx`: update `isStreaming` to `!!streamingResponse && !comment.agent_response && !isStreamingComplete`
- [x] In `DesignReviewContent.tsx`: pass `isStreamingComplete={isCurrentlyStreaming ? !!streamingResponse.isComplete : undefined}` to `InlineCommentBubble`
- [x] Add `isComplete?: boolean` to `StreamingResponse` interface in `CommentLogSidebar.tsx`
- [x] In `CommentLogSidebar.tsx`: update `isStreaming` to `isActiveStream && !comment.agent_response && !streamingResponse?.isComplete`
- [x] TypeScript: `tsc --noEmit` passes with zero errors

## Verified via Chrome MCP

The race condition was reproduced manually using React fiber state injection:

1. Set `streamingResponse = { commentId: 'cmt_test001', content: '...', entries: [...] }` (simulating mid-stream)
2. Clear `agent_response` in DB (simulating the cache lag window)
3. Fire completion

**OLD behavior** (`setStreamingResponse(null)`): Agent Response box disappears entirely from comment 1 — see `screenshots/04-BUG-old-behavior-null-shows-waiting.png`

**NEW behavior** (`setStreamingResponse(prev => { ...prev, isComplete: true })`): Agent Response box remains with the response content visible — see `screenshots/05-FIX-isComplete-content-preserved.png`

Once React Query invalidation resolves and `agent_response` populates from the DB, the component naturally transitions to using `comment.agent_response` directly and `isComplete` has no further effect.
