# Requirements: Fix "Agent Did Not Respond" After First Comment Completes

## Problem

In the comment queue, when the agent finishes responding to comment 1 and immediately begins streaming comment 2, comment 1's inline bubble incorrectly shows "Waiting for agent response..." (or the `[Agent did not respond...]` timeout text). The agent DID respond — the displayed state is wrong.

## Root Cause

`setStreamingResponse(null)` is called immediately when `interaction.state === "complete"` fires (in `session_update` or `interaction_update` WebSocket handlers). This clears the streaming display for comment 1 before the React Query cache has refreshed. Until `queryClient.invalidateQueries()` resolves and brings back `comment.agent_response`, `displayResponse` is falsy — so the bubble shows the waiting/error state.

Key files:
- `frontend/src/components/spec-tasks/DesignReviewContent.tsx` — calls `setStreamingResponse(null)` on completion (lines ~474, ~531)
- `frontend/src/components/spec-tasks/InlineCommentBubble.tsx` — `displayResponse = streamingResponse || comment.agent_response` (line 47)
- `frontend/src/components/spec-tasks/CommentLogSidebar.tsx` — same pattern (line 60)

## User Stories

**US1:** As a reviewer, when the agent responds to my first comment and moves on to the second, I see the agent's completed response on comment 1 (collapsed to last few lines) — not a spinner or error.

**US2:** As a reviewer, I can expand comment 1's completed response to read the full text, even while comment 2 is actively streaming.

## Acceptance Criteria

- [ ] After agent finishes responding to comment 1, comment 1 shows its response (last ~4 lines visible, expandable) while comment 2 is streaming.
- [ ] Comment 1 never flashes to "Waiting for agent response..." between completion and the cache refresh.
- [ ] Comment 1 does not display the streaming indicator or typing animation once complete.
- [ ] The expand/collapse toggle works on comment 1's completed response.
- [ ] Comment 2 streaming is unaffected — it displays correctly as before.
- [ ] Both `InlineCommentBubble` and `CommentLogSidebar` behave correctly.
