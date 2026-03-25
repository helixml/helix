# Design: Fix Comment Queue Transition State

## Architecture Pattern

The comment queue uses a single `streamingResponse` React state in `DesignReviewContent.tsx` to track the currently-streaming comment's content. When streaming completes, `setStreamingResponse(null)` is called immediately alongside `queryClient.invalidateQueries()`.

The race: the cache invalidation is async (network round-trip), so child components lose `streamingResponse` before `comment.agent_response` is available in the refreshed cache. Result: comment 1's `displayResponse` is falsy for 1–5 seconds → shows "Waiting for agent response...".

## Solution: Mark Streaming as Complete Instead of Clearing

Add an `isComplete` flag to the `streamingResponse` state shape. When the interaction finishes, set `isComplete: true` instead of clearing to `null`. This keeps comment 1's content visible (without the streaming indicator) until either:
1. Comment 2's first `interaction_patch` event arrives and naturally overwrites `streamingResponse` with comment 2's data, OR
2. The cache refresh confirms `comment.agent_response` is populated (at which point `comment.agent_response` takes over as the display source and `streamingResponse` for that comment is no longer needed)

### State Change

```typescript
// Current state shape
type StreamingState = { commentId: string; content: string; entries: ResponseEntry[] } | null

// New state shape
type StreamingState = {
  commentId: string;
  content: string;
  entries: ResponseEntry[];
  isComplete?: boolean;   // true = done streaming but content still displayed as fallback
} | null
```

### On Completion (lines ~462–477 and ~521–533 in DesignReviewContent.tsx)

```typescript
// Before (clears immediately):
setStreamingResponse(null);

// After (marks complete, keeps content visible):
setStreamingResponse(prev => prev ? { ...prev, isComplete: true } : null);
```

### Natural Cleanup

When the next comment's `interaction_patch` arrives, the handler calls:
```typescript
setStreamingResponse({ commentId: comment2Id, content: ..., entries: ... });
// isComplete is NOT set → comment 2 shows as streaming, comment 1's old state is gone
```
No explicit cleanup of the old comment 1 state is needed.

### Display Components

`InlineCommentBubble` and `CommentLogSidebar` both compute:
```typescript
const isActiveStream = streamingResponse?.commentId === comment.id
const displayResponse = isActiveStream ? streamingResponse!.content : comment.agent_response
const isStreaming = isActiveStream && !comment.agent_response
```

When `isComplete: true`, `isActiveStream` is still `true` for comment 1, so `displayResponse` shows the content. `isStreaming` is `false` once `comment.agent_response` is populated — but here we also want `isStreaming = false` even before the cache updates:

```typescript
const isStreaming = isActiveStream && !comment.agent_response && !streamingResponse?.isComplete
```

This ensures the streaming indicator disappears immediately when the response finishes, while the content stays visible.

## Key Files

| File | Change |
|------|--------|
| `frontend/src/components/spec-tasks/DesignReviewContent.tsx` | Change `setStreamingResponse(null)` to `setStreamingResponse(prev => prev ? { ...prev, isComplete: true } : null)` in both completion handlers |
| `frontend/src/components/spec-tasks/CommentLogSidebar.tsx` | Update `isStreaming` computation to check `!streamingResponse?.isComplete` |
| `frontend/src/components/spec-tasks/InlineCommentBubble.tsx` | Update `isStreaming` prop computation at call site in DesignReviewContent; or update internal `isStreaming` if `isComplete` is passed down |

## Passing isComplete to Child Components

`InlineCommentBubble` receives `streamingResponse?: string` (just the content string) and `streamingEntries`. It does not currently receive `isComplete`. Options:
- **Option A** (minimal): Pass `isStreaming` as an explicit prop computed in `DesignReviewContent` using the `isComplete` flag — this matches existing patterns and avoids changing child component interfaces unnecessarily.
- **Option B**: Pass the full `StreamingResponse` object (including `isComplete`) to the bubble.

Recommend **Option A**: Compute `isStreaming` in `DesignReviewContent` and pass it down, keeping child components simple.

## No Backend Changes Needed

The backend correctly populates `agent_response` via `updateCommentWithStreamingResponse()` during streaming and `finalizeCommentResponse()` at completion. The 2-minute timeout is guarded (`RequestID != "" && AgentResponse == ""`). This is a pure frontend display race.

## Codebase Pattern Notes

- This project uses React Query with `refetchInterval` for polling and `invalidateQueries` after mutations (per CLAUDE.md — no `setQueryData`).
- WebSocket streaming uses `accumulatedResponse` + `streamEntries` local variables (inside the `useEffect` closure) for real-time state; the `streamingResponse` React state is updated via `setStreamingResponse`.
- Both `session_update` (lines ~393–477) and `interaction_update` (lines ~481–533) completion handlers in `DesignReviewContent.tsx` need the same fix.
