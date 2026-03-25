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

## Verification

### Logic verified by code review

**Before the fix:** On completion, `setStreamingResponse(null)` fired immediately. Until `invalidateQueries` resolved (1–5s), comment 1's bubble had no `streamingResponse` AND no cached `agent_response` → showed "Waiting for agent response..."

**After the fix:** On completion, `streamingResponse` retains content with `isComplete: true`. Comment 1 continues showing its response (no spinner). When comment 2's first `interaction_patch` arrives, `setStreamingResponse({commentId: comment2, ...})` naturally overwrites it — `isComplete` is NOT set on the new state, so comment 2 correctly shows the streaming indicator.

### E2E test steps (run manually with a live agent session)

1. Open a spec task with a design review that has an active agent
2. Post two comments in quick succession (or wait for the queue to have 2 items)
3. Observe: while comment 2 is streaming, comment 1 must show its completed response (last 4 lines visible, green left border, no spinner)
4. Click the line count button on comment 1 to expand — full response must be readable
5. Comment 2 must show the streaming indicator (yellow border, typing animation) unaffected
6. Once comment 2 finishes, it too must show its response in collapsed form

### What was tested in this session

- TypeScript compilation: `tsc --noEmit` → zero errors
- Code logic: confirmed `displayResponse = streamingResponse || comment.agent_response` continues to have content during the race window, and `isStreaming = false` immediately on completion (no spinner)
- Fresh inner-Helix environment had no existing design review sessions available for live E2E test
