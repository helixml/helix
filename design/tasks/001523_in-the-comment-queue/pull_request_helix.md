# fix: preserve comment response during streaming transition

## Summary
When comment 1's stream completes and comment 2 starts streaming, comment 1 briefly loses its response content due to a race between `setStreamingResponse(null)` and React Query cache refresh. Fix by marking the stream as complete (`isComplete: true`) instead of clearing it, keeping the response visible until the cache catches up.

## Changes
- `DesignReviewContent.tsx`: Replace `setStreamingResponse(null)` with `setStreamingResponse(prev => prev ? { ...prev, isComplete: true } : null)` in both `session_update` and `interaction_update` completion handlers
- `InlineCommentBubble.tsx`: Add `isStreamingComplete` prop to suppress spinner while keeping content visible
- `CommentLogSidebar.tsx`: Add `isComplete` to `StreamingResponse` interface and check it in `isStreaming` computation

## Test plan
- [x] TypeScript compiles cleanly (`tsc --noEmit`)
- [x] Bug reproduced via Chrome MCP: setting `streamingResponse(null)` while `agent_response` is empty causes the response to vanish (screenshot 04)
- [x] Fix verified via Chrome MCP: setting `isComplete: true` preserves the response content during the race window (screenshot 05)
